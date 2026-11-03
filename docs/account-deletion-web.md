# 网页删除账号

`chat.tellomi.cn/delete-account/` 上的表单走本仓库的 Teamgram，不再调用已经下线的旧 `/v1` 服务。删除和 App 内 `account.deleteAccount` 是同一段清理：用户名释放、`users.deleted`、手机号改成不可再登录的占位、邮箱清空、全部会话作废。

## 流程

1. 网页 `POST /v1/verification/session`，body `{"phone":"+8613..."}`。号码按登录同一套规则归一化，默认只收 `+86`。
2. 短信走 `auth.sendCode` 用的同一发送器（`TELLOMI_SMS_PROVIDER=aliyun` 时是阿里云；未设置时开发验证码是 `12345`，不外发）。配额和登录共用 `TELLOMI_SMS_*`（手机号每天、IP 每天、重发间隔）。
3. 号码没有账号时，HTTP 响应和有账号时一样（都是 `session_id`）。不会发短信，也不会占用这个号码的短信配额。验证码填任何值都是 `code_invalid`。
4. `POST /v1/tellomi/account-deletion`，body 里带 `session_id`、`code`、`confirm`。`confirm` 必须是 `DELETE`。
5. 账号开了两步验证时，第一次提交返回 `password_required` 和密码提示。再提交时带上 `password`。服务端用现有 SRP 校验，明文密码不写日志。
6. 通过后调用 `account.deleteAccount` 的同一段清理。

会话令牌 10 分钟有效，删除成功后作废。验证码和密码各错 5 次后会话作废。

只接受 `Origin: https://www.tellomi.cn` 或 `https://chat.tellomi.cn`。

## 和 App 内删除的对应关系

[官方说明](https://core.telegram.org/api/account-deletion)：

| 情况 | 结果 |
| --- | --- |
| 没有云密码 | 立刻删除 |
| 有云密码，并且本次提交了正确密码 | 立刻删除 |
| 有云密码，没有提交密码，密码是 7 天前设的（或设置时间未知）且账号近 7 天使用过 | 延后 7 天，返回 `2FA_CONFIRM_WAIT_604800`。到期后同一段清理会执行。重新登录（`auth.signIn` 成功或 `auth.checkPassword` 成功）会取消 |
| 有云密码，没有提交密码，但密码是 7 天内设的，或账号近 7 天没用过 | 立刻删除 |

网页在开了两步验证时不会走「不填密码」这条，必须填密码。App 里不填密码的调用仍按上表处理，并给账号发一条服务通知：7 天内重新登录即可取消。

`password_set_at` 是后加的列，旧数据为 0，按「早于 7 天」处理，因此不填密码不会把老账号立刻删掉。

## 部署（成都 8.137.39.82）

不要在这台机器上做别的手工改库。新镜像的入口会先跑 `teamgramd/deploy/migrate.sh`，其中包含 `migrate-20261103-account-deletion.sql` 和 `migrate-20261103-account-wipe.sql`（队列表 `account_media_gc`）。

1. 按现有发版把包含本变更的镜像部署到 `teamgram` 容器（`docker-compose.yaml` 已把 `127.0.0.1:8802` 映射进容器的 `8802`）。进程监听 `TELLOMI_ACCOUNT_DELETE_ADDR`，默认 `0.0.0.0:8802`。设成 `off` 可关掉网页入口，App 内删除不受影响。
2. 在宿主机 nginx 的 `chat.tellomi.cn` TLS `server` 里加入 `teamgramd/deploy/nginx/chat.tellomi.cn.conf` 的三个 `location`。`X-Forwarded-For` 用 `$remote_addr`，不要把客户端自己带的地址拼进去。
3. `nginx -t && nginx -s reload`。
4. **上线前先备份 MySQL。** 删号会改消息、成员、资料，做完不能从应用里恢复。对象存储默认只记账（`TELLOMI_ACCOUNT_GC_DRY_RUN` 默认 true）。确认备份可以还原之后，再把这个变量设成 `false` 才会把文件移到 `trash/` 并在保留窗口后真删。
5. 本机检查：`curl -sI -H 'Host: chat.tellomi.cn' http://127.0.0.1:8802/delete-account/` 应是 `200`，`content-type` 为 `text/html`。不要把验证码或密码打进这条命令。

旧的静态目录先留着，不要删。nginx 把 `/delete-account/` 转到 8802 之后，磁盘上的旧文件不会再被访问。

## 回滚

1. 从 nginx 的 `server` 里去掉上述三个 `location`，`nginx -t && nginx -s reload`。若旧静态文件还在原来的 root 下，`/delete-account/` 会回到 2026-09-26 的页面；它调用的 `/v1` 会再次 502，直到你把 location 加回来。
2. 若新进程本身有问题：把 `teamgram` 镜像退回上一版并重启。`password_set_at`、`account_deletion_pending` 和 `account_media_gc` 可以留在库里，旧程序不读它们。已经删掉的私聊和已经清空的资料不会因为退回镜像而回来，所以要靠上线前的 MySQL 备份。
3. 只想停网页、保留 App 内删除：给容器加上 `TELLOMI_ACCOUNT_DELETE_ADDR=off` 后重启，并按第 1 步卸掉 nginx location。

## 删号后还在不在

App 内 `account.deleteAccount` 和网页删号走同一段 `PurgeAccount`。用户行保留（`deleted=1`、名字清空），这样群里的旧消息还能显示「已删除账号」。这一步**不可恢复**。上线前备份 MySQL。

群主处理：先转给最早加入且仍在群里的管理员（加入时间相同则取较小的 user id）；没有管理员就转给最早的普通成员；然后把该用户标成已离开并减少成员数。只剩群主一人时，把群或频道标成停用（普通群 `chats.deactivated=1`，频道 `channels.deleted=1`），**不删消息行**。原因：Teamgram 里群主直接退出会得到 `USER_CREATOR`，`channels.editCreator` / `messages.editChatCreator` 又要求云密码，删号时问不了这个密码。不转交的话，剩下的成员谁都不能再设管理员。没有第二个人时，没有可转交的对象，停用比留下一个无法管理、也没有成员的群更干净，历史仍留在表里。

| 数据类别 | 存放位置 | 删号后 | 源码 |
| --- | --- | --- | --- |
| 用户行 | MySQL `users` | 还在。`deleted=1`，手机号改成 `-{id}`，邮箱 `NULL`。名字、姓、简介、生日、`username`、`photo_id` 清空。客户端把这行显示成已删除账号 | `wipe.SQL.ClearProfile`，然后 `UsersDAO.Delete` |
| 用户名 | MySQL `username` 和 `users.username` | 不在。两处都删，用户名可以再注册 | `ClearProfile`，以及原来的 `UserDeleteUsername` |
| 头像记录 | MySQL `user_profile_photos` | 不在（`deleted=1`）。文件见下面的对象存储 | `ClearProfile` |
| 私聊消息 | MySQL `messages`（按用户分表时是 `messages_N`） | 不在。该用户和对方盒子里 `deleted=1`，每批 200 条，各自提交。在线对方收到 `updateDeleteMessages`（带 pts，并写入 `user_pts_updates`） | `wipe.Engine.wipePeer` |
| 私聊对话 | MySQL `dialogs` | 不在。双方 `deleted=1`，对话列表缓存键删掉。层 232 没有单独的「对话已删除」更新，对方靠 `updateDeleteMessages` 清掉消息，下次拉对话列表时这一行已经没了 | `DeleteDialog` |
| 自己的云收藏（Saved Messages） | 同上，`peer_type=1` | 不在 | 同上 |
| 秘密聊天 | 进程内 `secretchat.Store` | 不在。双方收到 `updateEncryption`（`encryptedChatDiscarded`，`history_deleted`）。重启前还在内存里的密文和加密文件 id 会进清理队列。进程重启后内存里的秘密聊天本来就会丢 | `secretchat.Store.DiscardAll` |
| 普通群 / 超级群 / 频道成员 | `chat_participants`、`channel_participants`、成员计数 | 不在。状态改为已离开，计数减少，版本加一。其余在线成员收到 `updateChatParticipantDelete` 或频道服务发出的退出更新 | `LeaveBasic`、`channel.LeaveDeletedAccount` |
| 群和频道里该用户发过的消息 | `messages`（`peer_type=3`）、`channel_messages` | **还在。** 不标删除。发送者仍是原来的 user id，用户行是已删除账号 | 删号路径不更新这些行 |
| 群主 | `chats.creator_user_id`、`channels.creator_user_id` | 转给最早的管理员，否则最早的普通成员。只剩一人则群/频道停用，消息还在 | `PickSuccessor` |
| 联系人 | `user_contacts` | 不在列表里。该用户自己的行，以及别人通讯录里指向他的行，都是 `is_deleted=1`、`mutual=0`（和 `DeleteUserContact` 一样用软删除，不是物理 DELETE） | `DeleteContacts` |
| 两步验证 | `user_passwords`、`user_password_srp`、`user_password_login` | 不在 | `DeleteSettings` |
| 隐私、通知、拉黑、设备推送令牌、对话文件夹 | `user_privacies`、`user_global_privacy_settings`、`user_notify_settings`、`user_peer_settings`、`user_peer_blocks`、`user_settings`、`devices`、`dialog_filters` | 不在（删行或 `deleted=1`） | `DeleteSettings` |
| 授权会话 | MySQL `auth_users`，Redis `auth_data.2#…` | 不在。和原来一样作废全部会话 | `AuthsessionResetAuthorization` |
| 资料缓存 | Redis `user_data.2#`、`phone_user.1#`、`user_contact.2#`、`dialog.1#`、`chat#` | 删掉这些键，下次读库 | `cacheKeys` |
| 7 天待删 | `account_deletion_pending` | 立刻删除时 `Cancel`。到期后 reaper 跑同一段清理 | `deletion/pending.go` |
| 只被已删除私聊引用的照片、文件、加密文件、头像对象 | 队列表 `account_media_gc`，对象在 S3 | 异步。仍被未删除的群消息、频道消息、别人的头像或贴纸引用的留着。默认 dry-run，只打日志。关掉 dry-run 后先复制到同一桶的 `trash/` 前缀并删掉原键，`TELLOMI_ACCOUNT_GC_KEEP_DAYS`（默认 30）天后再删 trash 里的副本 | `wipe.RunGC`、`MinioUtil.MoveToTrash` |
| 贴纸、emoji、壁纸 | public-media 桶 | **还在。** 清理拒绝这些 class | `trashClass` |
| 90 天媒体保留 | 另一条任务 | 还在跑，和删号队列互不替代。删号把私聊标成 deleted 之后，不再被引用的聊天媒体才会被那条任务看成可过期 | `docs/media-retention.md` |

数据库更新按批提交（消息每批 200 行，联系人每批 500 行），条件里带 `deleted=0` 或 `state=0`，中途失败再跑同一段不会重复减成员数，也不会把已经删掉的消息再删一遍。大账号不会用一条语句锁住整张消息表。

### 对象存储：生产写的是 R2

清理走 `objectstore.Client`，也就是 dfs 用的那一个 `TELLOMI_S3_*` 客户端（`minio-go`）。它打到环境变量里的 endpoint，不写死某一种后端。每一轮 GC 日志都带这个 endpoint，不带密钥。

成都生产实际写入的是 **Cloudflare R2**，不是那台机器上可能还在跑的 MinIO 容器。依据是仓库配置，不是登录生产机看出来的：

- `docs/ci-acr.md` 写明 dfs 已经通过 `TELLOMI_S3_*` 使用 R2，并要求部署时不要加 `--profile local-minio`。`minio` / `minio-mc` 不在默认拉取列表里，`teamgram` 也不 `depends_on` MinIO。
- `docker-compose.yaml` 里的 `TELLOMI_S3_ENDPOINT:-http://minio:9000` 只是变量没设时的本地默认。生产 `.env` 按 `docs/storage.md` 和 `docs/ci-acr.md` 指向 `https://<accountid>.r2.cloudflarestorage.com`。
- 同一台机器上的 MinIO 容器只有在 `TELLOMI_S3_ENDPOINT` 仍指向它时才会收到读写。文档要求生产不要这样配。若有人把生产变量改回 MinIO，清理会跟着打到 MinIO，因为用的是同一个客户端。

默认 `TELLOMI_ACCOUNT_GC_DRY_RUN=true`（变量不设也是 true）。设成 `false` 才会移动和删除对象。保留天数是 `TELLOMI_ACCOUNT_GC_KEEP_DAYS`，默认 30。消息分表数量是 `TELLOMI_MESSAGE_SHARDING`，默认 1，即表名 `messages`。

## 还没做的

- 7 天等待期内，取消方式是重新登录。官方还有一条「点开服务消息里的手机确认链接」的路径，这里没有做。
- 短信签名和模板继续用现有的 `TELLOMI_ALIYUN_SMS_*`。网页不会单独申请模板。
- 秘密聊天的状态只在 bff 进程内存里。删号时会丢掉当前进程还记着的那些；更早一次重启已经丢掉的，没有行可删。

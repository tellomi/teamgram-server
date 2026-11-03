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

不要在这台机器上做别的手工改库。新镜像的入口会先跑 `teamgramd/deploy/migrate.sh`，其中包含 `migrate-20261103-account-deletion.sql`。

1. 按现有发版把包含本变更的镜像部署到 `teamgram` 容器（`docker-compose.yaml` 已把 `127.0.0.1:8802` 映射进容器的 `8802`）。进程监听 `TELLOMI_ACCOUNT_DELETE_ADDR`，默认 `0.0.0.0:8802`。设成 `off` 可关掉网页入口，App 内删除不受影响。
2. 在宿主机 nginx 的 `chat.tellomi.cn` TLS `server` 里加入 `teamgramd/deploy/nginx/chat.tellomi.cn.conf` 的三个 `location`。`X-Forwarded-For` 用 `$remote_addr`，不要把客户端自己带的地址拼进去。
3. `nginx -t && nginx -s reload`。
4. 本机检查：`curl -sI -H 'Host: chat.tellomi.cn' http://127.0.0.1:8802/delete-account/` 应是 `200`，`content-type` 为 `text/html`。不要把验证码或密码打进这条命令。

旧的静态目录先留着，不要删。nginx 把 `/delete-account/` 转到 8802 之后，磁盘上的旧文件不会再被访问。

## 回滚

1. 从 nginx 的 `server` 里去掉上述三个 `location`，`nginx -t && nginx -s reload`。若旧静态文件还在原来的 root 下，`/delete-account/` 会回到 2026-09-26 的页面；它调用的 `/v1` 会再次 502，直到你把 location 加回来。
2. 若新进程本身有问题：把 `teamgram` 镜像退回上一版并重启。`password_set_at` 和 `account_deletion_pending` 可以留在库里，旧程序不读它们。
3. 只想停网页、保留 App 内删除：给容器加上 `TELLOMI_ACCOUNT_DELETE_ADDR=off` 后重启，并按第 1 步卸掉 nginx location。

## 删号实际清掉什么

App 内 `account.deleteAccount` 和网页删号走同一段 `PurgeAccount`。对照源码，它**不是**「删掉这个用户的全部数据」，对象存储（生产是 R2，本地开发是 MinIO）里的文件一次都不会被删。

官方 Telegram（[core.telegram.org/api/account-deletion](https://core.telegram.org/api/account-deletion)，以及客户端 FAQ）的产品语义是：账号不能再登录，用户名可被重新注册，私聊记录从双方设备消失，你被移出群组，别人看到的是已删除账号。本仓库没有做到这一步。发出去的消息、群里的媒体、R2 上的对象都还在。

| 数据类别 | 存放位置 | 删号后 | 源码 |
| --- | --- | --- | --- |
| 用户行（名字、简介、头像 id 等） | MySQL `users` | 软删除：`deleted=1`，`delete_reason` 写入。名字和简介列不改 | `UsersDAO.Delete`：`update users set phone = ?, email = NULL, deleted = 1, delete_reason = ?` |
| 手机号 | MySQL `users.phone` | 不留原号。改成 `-{用户 id}`，原号可以再注册 | 同上，`Dao.DeleteUser` 传入 `"-"+id` |
| 邮箱 | MySQL `users.email` | 置 `NULL` | 同上 |
| 用户名索引 | MySQL `username` | 硬删除这一行，用户名可以再被占用 | `UsernameDAO.Delete`：`delete from username where username = ?` |
| `users.username` 列 | MySQL `users` | 不更新，行里可能还留着旧字符串；登录查找走的是 `username` 表 | `account.deleteAccount_handler.go` 只调 `UserDeleteUsername` |
| 授权会话 | MySQL `auth_users` | 软删除：`deleted=1`，`date_active=0`。`auth_keys` 密钥行不删 | `AuthUsersDAO.DeleteByHashList`、`DeleteUser`；`Dao.ResetAuthorization`、`UnbindAuthUser` |
| 会话 Redis | Redis `auth_data.2#{authKeyId}` | 删除这些键 | `genAuthDataCacheKey`，随上面的 `CachedConn.Exec` |
| 用户资料 Redis | Redis `user_data.2#{id}`、`phone_user.1#{原手机号}` | 删除这两个键。其它用户缓存不扫 | `Dao.DeleteUser` |
| 联系人 | MySQL `user_contacts` 等 | 不删。对方通讯录里的 user id 还在，资料接口会看到 `deleted` | 删号路径没有联系人 SQL |
| 私聊消息、自己的会话列表 | MySQL `messages`、`dialogs` | 不删。发给对方的那一份还在对方盒子里 | 删号路径没有消息 SQL |
| 群消息、群成员 | MySQL `messages`、`chat_participants`、`channel_participants` | 不删，也不会退群。群里的消息仍挂在原来的 sender id 上 | 同上 |
| 两步验证 | MySQL `user_passwords`、`user_password_srp`、`user_password_login` | 不删。账号已不能登录，但校验器还在表里 | 删号路径不调用 password store |
| 7 天待删记录 | MySQL `account_deletion_pending` | 立刻删除时会 `Cancel` 掉。到期由 bff 里的 reaper 再跑同一段 `PurgeAccount` | `deletion/pending.go` |
| 头像、照片、视频、语音、文件 | MySQL `photos` / `documents` / `photo_sizes`，对象在 S3 | 不删。行还在，对象还在 | 删号路径不调用 `objectstore.Client.Delete` |
| 对象存储后端 | 生产 Cloudflare R2；本地 Docker MinIO。同一套 `TELLOMI_S3_*`，见 `docs/storage.md`、`docker-compose.yaml` | 删号不调用 `RemoveObject`。`objectstore.Client.Delete` 只给媒体保留任务用，且拒绝删公开媒体类（头像、贴纸） | `app/service/dfs/objectstore/client.go`、`app/service/dfs/internal/minio_util/minio.go` |
| 90 天媒体保留 | 对象键见 `docs/storage.md` 的前缀表（`photos/`、`documents/`、`videos/` 在 attachments 桶；`avatars/` 在 public-media 桶） | 与删号无关。只删「上传已超过保留天数、且没有任何未删除消息或在群成员还引用」的聊天媒体。删号不把消息标成 deleted，所以这些对象仍被引用，清扫不会拿走。头像、贴纸、GIF、壁纸本来就不在清扫范围内 | `docs/media-retention.md`、`app/service/media/retention` |

发给别人的私聊消息和已经发到群里的媒体，删号后都还在。对方仍能按原来的 document/photo id 从 R2 下载，直到 90 天保留规则在「没有任何有效引用」时另行清掉聊天媒体。头像不在那条规则里，会一直留在 R2。

### 对象存储清理方案（先不实现）

不要在 `PurgeAccount` 里同步删 R2。消息和头像被很多行共享，同步删会把别人的转发和群文件一起删掉，也没有回滚。

1. 删号成功后只写一张队列表 `account_media_gc`（user_id、阶段、创建时间）。bff 里已有的 reaper 风格再加一个异步任务，按用户分批跑，失败留下一行下轮再试。
2. 引用计数按对象 id，不按用户。照片、文档、视频的 id 出现在：`messages`（含对方的盒子和群）、`photos`/`documents` 被其它用户头像或贴纸集引用、stories。计数大于 0 的对象不删。只被这个已删除用户引用、且产品决定「对方私聊也要消失」时，才把那些消息标成 deleted 并让计数降到 0。
3. 群和频道里的文件默认不删：发送时已经复制到群的消息行上，官方语义也是群消息还在、发送者显示为已删除账号。头像若只挂在 `users.photo` 上，可以删 `avatars/` 前缀；若同一文件还被某条消息引用，则留着。
4. 贴纸、emoji、壁纸是公共目录，不因为某个用户删号而删。
5. 可回滚：先把对象复制或改名到 `trash/{user_id}/...`，MySQL 只记 `gc_status=trashed`。保留窗口（建议与下面要确认的消息保留时长相同，例如 30 天）内可以把键名改回去。窗口过后才 `RemoveObject`。任务本身用 `dry_run` 开关，和现有 `TELLOMI_MEDIA_RETENTION_DRY_RUN` 一样，默认先只记账不删。
6. `objectstore.Client.Delete` 今天拒绝公开媒体类。头像要另开一个只允许 `avatars/` 的删除入口，避免误删贴纸桶。

这个方案没有写进本次代码。

## 需要确认

- 删除后聊天记录、媒体、联系人仍留在库里。现行 `users` 删除只是标记 `deleted`、换掉手机号、清空邮箱、释放用户名、踢掉会话。若要在若干天后物理删除消息，需要另定保留时长。
- 7 天等待期内，取消方式是重新登录。官方还有一条「点开服务消息里的手机确认链接」的路径，这里没有做。
- 短信签名和模板继续用现有的 `TELLOMI_ALIYUN_SMS_*`。网页不会单独申请模板。

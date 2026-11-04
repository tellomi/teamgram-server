# Tellomi 接口缺口清单

对照客户端当前 TL layer，盘点本仓库（Teamgram）上登录、启动和日常会打到的 MTProto 方法。本文件说明哪些已经能用、哪些在本 PR 补上、哪些还留着。

## 客户端 layer

| 端 | 仓库 | 提交 | Layer |
| --- | --- | --- | --- |
| iOS | `tellomi/Telegram-iOS` | `6bdc72e`（2026-11-03，登录注册重做） | `Serialization.currentLayer()` = **232** |
| Android | `tellomi/Telegram` | `a6345d6`（2026-11-03，登录注册重做） | `TLRPC.LAYER` = **232** |

iOS `submodules/TelegramApi/swift_scheme.tl` 与本仓库 `proto/schema/telegram_api_layer232.tl` 的函数集合一致，都是 **1201** 个。没有「客户端有、schema 没有」的方法。

调用点：iOS 扫的是 TelegramCore / AuthorizationUI 里的 `Api.functions.*`（759 个方法，其中 758 个在覆盖矩阵里，多出来的 `help.test` 是握手探测）。Android 登录页 `LoginActivity` 的两步验证走 `TL_account.getPassword` + `TL_auth_checkPassword`，和 iOS 同一套。服务端状态用 `docs/api-coverage.md` 的分类（路由 + handler），再对登录链路读了 handler。

分类：

| 状态 | 含义 |
| --- | --- |
| 已实现 | 会话路由到 bff，handler 读写真实状态 |
| 残缺或假数据 | 返回空对象、`Bool` 或 `*NotModified`，客户端当成功但没有数据 |
| 未实现 | `METHOD_NOT_IMPL` 或 `ENTERPRISE_IS_BLOCKED` |

## 数量

Tellomi iOS 实际调用、且落在覆盖矩阵里的 755 个方法：

| 优先级 | 已实现 | 未实现（返回错误） | 残缺或假数据 |
| --- | ---: | ---: | ---: |
| P0 登录 / 启动 / 后台同步 | 91 | 0 | 0 |
| P1 资料、联系人、私聊、历史上传、基础群 | 352 | 8 | 0 |
| P2 其余（动态、支付、机器人、商业等） | 304 | 0 | 0 |

P0 包含 `auth.sendCode` / `auth.signIn` / `auth.checkPassword` / `account.getPassword`、`help.getConfig`、`updates.getState` / `updates.getDifference`、`messages.getDialogs`、`langpack.getLangPack`。这些都有路由和真实 handler，不是空壳。

微博 / 微信 / QQ / Apple / Google 登录、密码登录、付费接口在客户端方案里大多仍在 schema 中，但 Tellomi 登录流程不走它们。按 P2 留着，本 PR 不动。

## 本 PR 补上的

### P0：手机号 + 短信 + 两步验证，iOS 首次登录

现象：iOS 第一次用手机号、短信验证码、两步验证密码进不去；先用邮箱登录，再「添加账号」登录同一个手机号可以。

服务端原因（不是客户端把密码算错）：

1. 首次连接用的是临时授权密钥。`auth.bindTempAuthKey` 完成前，网关把后续 RPC 的永久密钥 id 填成 0。
2. 会话进程于是为「密钥 0」建了一个所有未绑定连接共用的会话。`auth.signIn` 仍返回 `SESSION_PASSWORD_NEEDED`（账号确实有云密码），但 `user_password_login` 写不进去（密钥为 0 直接跳过）。
3. `account.getPassword` 这时用户 id 是 0，返回的 `account.password` **没有** `current_algo` / `srp_B` / `srp_id`。
4. iOS `verifyPassword`（`TelegramCore/Sources/Account/Account.swift`）看到没有当前算法就失败，而且 **不会** 像 Android 那样在 `SRP_ID_INVALID` 时重新拉一次 `account.getPassword`。临时密钥和永久密钥各打一次时，旧逻辑还会因为密钥不一致丢掉挑战。
5. 「添加账号」发生在应用已经跑起来、临时密钥已经绑到永久密钥之后。会话是那把永久密钥，`account.getPassword` 能给出 SRP，`auth.checkPassword` 能过。

本 PR：

- 未绑定的临时密钥不再进入密钥 0 的会话。永久密钥行上 `perm_auth_key_id = 0` 时，按这把密钥自己的 id 处理（`BoundPermAuthKeyId`）。
- 没有永久密钥时，`auth.signIn` / `auth.sendCode`（含邮箱 future-auth-token）不再返回 `SESSION_PASSWORD_NEEDED`，而是 `AUTH_KEY_PERM_EMPTY`，避免客户端打开一个永远算不出 SRP 的密码页。
- 同样情况下 `account.getPassword` 也不再伪装成「这个号没有云密码」。
- `auth.checkPassword` 只校验 `srp_id` 和 M1，不再因为两次调用的 auth key 不同而回 `SRP_ID_INVALID`。`srp_id` 仍是一次性的。

相关代码：`app/interface/gnetway/internal/server/gnet/server_gnet.go`、`app/interface/session/internal/core/core.go`、`app/service/authsession/internal/dao/auth_key.go`、`app/bff/authorization/internal/core/auth.signIn_handler.go`、`app/bff/twofa/password/service.go`。

没有新的数据库表。`user_password_login` 仍是原来的迁移 `teamgramd/deploy/sql/migrate-20261101-cloud-password.sql`。

## 设备授权设置

`account.changeAuthorizationSettings` 已落库，不再返回 `ENTERPRISE_IS_BLOCKED`。返回 `boolTrue`。未知的 `hash` 返回 `400 HASH_INVALID`。`hash` 为 0 时改的是当前这把永久密钥（`account.getAuthorizations` 会把当前会话的 hash 写成 0）。没带上的 `Bool` 不改原来的值。`confirmed` 把该会话的 `unconfirmed` 清掉。

`account.getAuthorizations` 把这三项读回来：`encrypted_requests_disabled`、`call_requests_disabled`、`unconfirmed`。用户已经有别的在线会话时，新插入的 `auth_users` 行从 `unconfirmed` 开始；第一把会话不是。满 7 天（`authorization_autoconfirm_period`，`help.getAppConfig` 里也是 604800）之后，列表里不再标未确认。登录时给其他设备推 `updateNewAuthorization`，以及按会话丢掉来电或加密对话，这次没有做。

迁移：`teamgramd/deploy/sql/migrate-20261103-authorization-settings.sql`。容器入口会先跑 `teamgramd/deploy/migrate.sh`（先装 `0_migrate_helpers.sql`，再按文件名顺序执行还没记入 `schema_migrations` 的 `migrate-*.sql`）。手工：

```bash
mysql ... teamgram < teamgramd/deploy/sql/0_migrate_helpers.sql
mysql ... teamgram < teamgramd/deploy/sql/migrate-20261103-authorization-settings.sql
```

可以重复执行。列还没加上时，设备列表仍能打开（这三项当关），`account.changeAuthorizationSettings` 返回 `500 INTERNAL_SERVER_ERROR`。

## 邀请链接

iOS「通过链接邀请加入群组」会先读 `channelFull.exported_invite` / `chatFull.exported_invite`。没有主链接时再调 `messages.exportChatInvite`，`legacy_revoke_permanent` 置位。这个调用以前只接受基础群，超级群和频道直接 `PEER_ID_INVALID`，分享页的进度条就不会停。「管理邀请链接 → 新链接」带 `title`、`request_needed`、`expire_date`、`usage_limit`，同样因为 peer 不是基础群而失败，客户端弹出「发生错误，请稍后再试」。

现在这组方法对基础群、超级群和频道都按 layer 232 处理：

- `messages.exportChatInvite`（含 `legacy_revoke_permanent`、`expire_date`、`usage_limit`、`request_needed`、`title`。`subscription_pricing` 返回 `406 PAYMENT_UNSUPPORTED`，本服务不卖 Stars 订阅）
- `messages.getExportedChatInvites`、`messages.getExportedChatInvite`
- `messages.editExportedChatInvite`（含 `revoked`。撤销主链接时返回 `messages.exportedChatInviteReplaced`）
- `messages.deleteExportedChatInvite`、`messages.deleteRevokedExportedChatInvites`
- `messages.getChatInviteImporters`、`messages.getAdminsWithInvites`
- `messages.checkChatInvite`、`messages.importChatInvite`（`#de91436e` 和旧的 `#6c50051c`）
- `messages.hideChatJoinRequest`、`messages.hideAllChatJoinRequests`
- `messages.getFullChat` 和 `channels.getFullChannel` 的 `exported_invite`

链接是 `https://tell.cc/+` 加 20 位 hash（`TELLOMI_LINK_BASE_URL` / `TEAMGRAM_TME_HOST` 可改主机）。基础群写 `chat_invites` / `chat_invite_participants`，频道和超级群写 `channel_invites` / `channel_invite_importers`。迁移：`teamgramd/deploy/sql/migrate-20261104-chat-invites.sql`。旧库的 `chat_invite_participants` 缺 `chat_id`、`requested`、`approved_by`，加入记录和次数统计会插不进去。

## 常见路径里还缺的

对照 iOS（`tellomi/Telegram-iOS`）和 Android（`tellomi/Telegram`）会在群、频道、联系人、隐私、设置里打到的方法。覆盖矩阵把它们标成已实现，是因为基础群或私聊走得通。下面这些分支仍然返回 `ENTERPRISE_IS_BLOCKED`：

| 方法 | 会在哪碰到 | 处理 |
| --- | --- | --- |
| `messages.editChatAbout` | 频道 / 超级群简介 | 另开 PR，基础群已经能改 |
| `messages.saveDraft` / `messages.clearAllDrafts` | 频道草稿 | 另开 PR，私聊和基础群已经能存 |
| `account.updateNotifySettings` | 频道免打扰 | 另开 PR，用户和基础群已经能改 |
| `messages.deleteHistory` | 清空超级群自己的记录 | 另开 PR。`channels.deleteHistory` 是另一条已经实现的路径 |
| `users.getIsPremiumRequiredToContact` | 打开资料页时的隐私判断 | 另开 PR |
| `messages.forwardMessages` | 从频道把帖子转发走 | 留下。私聊和基础群转发是通的 |
| `upload.getFile` 的贴纸集缩略图 | 贴纸面板 | 留下。普通文件和秘密聊天文件走 DFS |
| `account.toggleSponsoredMessages`、`channels.restrictSponsoredMessages`、`messages.viewSponsoredMessage`、`messages.clickSponsoredMessage`、`messages.reportSponsoredMessage` | 赞助消息 | 留下。Tellomi 没有广告 |
| `messages.composeRichMessageWithAI`、`messages.translateRichMessage` | AI 撰写 / 富文本翻译 | 留下。普通 `messages.translateText` 另有实现 |
| `account.confirmBotConnection` | 机器人连接确认 | 留下 |
| `auth.importBotAuthorization`、`auth.importWebTokenAuthorization`、Firebase PNV、`auth.checkPaidAuth` | 机器人、网页令牌、付费登录 | 留下。Tellomi 登录不走这些 |

## 留下的 P1（客户端会调，服务端返回错误）

下面这 8 个仍不在登录和聊天列表的必经路径上。

| 方法 | 客户端 | 现状 | 说明 |
| --- | --- | --- | --- |
| `account.confirmBotConnection` | iOS | `ENTERPRISE_IS_BLOCKED` | 机器人连接确认 |
| `account.toggleSponsoredMessages` | iOS、Android | `ENTERPRISE_IS_BLOCKED` | 赞助消息开关。Tellomi 未做广告 |
| `channels.restrictSponsoredMessages` | iOS、Android | `ENTERPRISE_IS_BLOCKED` | 频道赞助消息 |
| `messages.viewSponsoredMessage` | iOS、Android | `ENTERPRISE_IS_BLOCKED` | 同上 |
| `messages.clickSponsoredMessage` | iOS、Android | `ENTERPRISE_IS_BLOCKED` | 同上 |
| `messages.reportSponsoredMessage` | iOS、Android | `ENTERPRISE_IS_BLOCKED` | 同上 |
| `messages.composeRichMessageWithAI` | iOS、Android | `ENTERPRISE_IS_BLOCKED` | AI 撰写 |
| `messages.translateRichMessage` | iOS、Android | `ENTERPRISE_IS_BLOCKED` | AI 翻译富文本。普通 `messages.translateText` 另有实现 |

`messages.uploadEncryptedFile` 已实现。客户端先用 `upload.saveFilePart` / `upload.saveBigFilePart` 传分片，再把 `inputEncryptedFileUploaded` 或 `inputEncryptedFileBigUploaded` 交给这个方法。服务端把分片收成 `encryptedFile`（含 `key_fingerprint`），对象写在 DFS 的 `encryptedfiles/<id>.dat`。对方用 `upload.getFile` 的 `inputEncryptedFileLocation` 读取。`inputEncryptedFile` 表示这份已经传过的文件。

## 不算缺口、但只做了一半的路径

这些方法整体是已实现。个别分支在频道或企业插件缺失时返回 `ENTERPRISE_IS_BLOCKED`。私聊和基础群走得到：

- `messages.saveDraft` / `messages.clearAllDrafts`：频道草稿被挡住，私聊和基础群可以存。
- `messages.editChatAbout`：基础群可以改简介，频道不行。
- `account.updateNotifySettings`：用户、基础群可以；频道分支要插件。
- `upload.getFile`：普通图片、文件，以及秘密聊天的 `inputEncryptedFileLocation` 走 DFS。贴纸集缩略图在没有插件时失败。
- `messages.forwardMessages` / `messages.deleteHistory`：有一条频道或加密分支返回企业错误，普通对话的转发和清空历史是实现了的。

频道、付费、Apple/Google 登录按 P2 留着。

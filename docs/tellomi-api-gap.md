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
| P1 资料、联系人、私聊、历史上传、基础群 | 360 | 0 | 0 |
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

## 这次补上的

聊天里客户端还会打到、并且以前直接 `ENTERPRISE_IS_BLOCKED` 的三个 P1 方法。登录、会话路由和鉴权没改。没有新的数据库表，也没有对 Redis 做 `FLUSHALL`。

### `messages.translateRichMessage`

翻译按钮在富文本消息上走这个方法，不走 `messages.translateText`。两条都调用百炼，并且都要 `TELLOMI_TRANSLATE=1` 和 `TELLOMI_AI_API_KEY`。`to_lang` 不在语言表里是 `400 TO_LANG_INVALID`；带了 `text` 就翻这几段，否则用 `peer` + `id` 读已发出的消息。空 id 列表是 `MESSAGE_IDS_EMPTY`，找不到的 id 是 `MESSAGE_ID_INVALID`，peer 不对是 `PEER_ID_INVALID`。`tone` 超过 64 个字符同样是 `TO_LANG_INVALID`。开关关掉、没有 key、或模型失败是 `500 TRANSLATE_REQ_FAILED`，不把原文当译文返回。

主模型 `qwen-plus`，失败或超时再换 `deepseek-v3`（仍走百炼）。百炼内容安全拦住时不换模型。`TELLOMI_CONTENT_SAFETY` 打开时，译文再过一遍文本审核，拦住也是 `TRANSLATE_REQ_FAILED`。块结构原样保留，只改叶子上的文字；代码块不改。HTML / Markdown 里的粗体和链接留在结果里。已发出的消息上的 entity 按 UTF-16 跟着译文移动。结果是 `messages.translatedRichMessage`，照片和文件向量是空的。`messages.translateText` 用同一套模型和同一把开关。

### `messages.composeRichMessageWithAI`

输入栏的 AI 按钮。`TELLOMI_AI_COMPOSE=1` 并且有 `TELLOMI_AI_API_KEY` 时，`help.getAppConfig` 的 `tellomi_ai_compose_enabled` 才是 true，校对、加表情、语气和 `translate_to_lang` 都调用上面的百炼模型。两个条件缺一，或者模型失败，是 `500 AICOMPOSE_TIMEOUT`，不把原文还回去。空文本是 `MESSAGE_EMPTY`，没指定任务是 `AI_COMPOSE_TASK_MISSING`，不认识的语气是 `AICOMPOSE_TONE_INVALID`。不返回 `AICOMPOSE_FLOOD_PREMIUM`。这块不依赖 `TELLOMI_AI`。

### `account.confirmBotConnection`

用户确认连上一个机器人时，客户端等的是 `Bool`。`bot_id` 为空、不是机器人、或者 access hash 对不上，返回 `400 BOT_ID_INVALID`。未登录是 `401 AUTH_KEY_UNREGISTERED`。`bots.createBot` 建在本进程里的机器人按返回的 access hash 核对。用户服务里 `is_bot` 的账号走 `user.isBot`，hash 不一致也是 `BOT_ID_INVALID`。确认写入进程内存，账号缓存可用时再写 `botconn:<用户>:<机器人>`。再确认一次仍是 `boolTrue`。不向会话推 `updateNewBotConnection`，避免一条客户端没在这次调用里等待的更新把连接打乱。

## 常见路径里还缺的

频道简介、草稿、免打扰、清空自己的历史、从频道转发、贴纸集封面、赞助消息的空结果，以及上面这三项，都有路由和真实 handler。`users.getIsPremiumRequiredToContact` 见下面「资料页」。

覆盖矩阵里，客户端会调并且仍可能返回错误的登录类方法没有改：`auth.importBotAuthorization`、`auth.importWebTokenAuthorization`、Firebase PNV、`auth.checkPaidAuth`。Tellomi 的手机号和邮箱登录不走这些。

## 留下的 P1（客户端会调，服务端返回错误）

上面三个 P1 不再返回 `ENTERPRISE_IS_BLOCKED`。赞助消息的读取、点击、查看、举报和开关已经返回空结果或成功。登录和聊天列表的必经路径没有新的错误桩。

`messages.uploadEncryptedFile` 已实现。客户端先用 `upload.saveFilePart` / `upload.saveBigFilePart` 传分片，再把 `inputEncryptedFileUploaded` 或 `inputEncryptedFileBigUploaded` 交给这个方法。服务端把分片收成 `encryptedFile`（含 `key_fingerprint`），对象写在 DFS 的 `encryptedfiles/<id>.dat`。对方用 `upload.getFile` 的 `inputEncryptedFileLocation` 读取。`inputEncryptedFile` 表示这份已经传过的文件。

## 资料页

`users.getIsPremiumRequiredToContact` 对每个 id 返回 `boolFalse`。Tellomi 不用 Premium 才能发消息这道门槛。

## 还留着的

- 输入栏 AI 默认关。`TELLOMI_AI_COMPOSE` 不是 `1`，或没有百炼 key 时，`tellomi_ai_compose_enabled` 是 false，调用返回 `AICOMPOSE_TIMEOUT`。
- `auth.importBotAuthorization`、`auth.importWebTokenAuthorization`、Firebase PNV、`auth.checkPaidAuth`：机器人、网页令牌、付费登录。Tellomi 的手机号和邮箱登录不走这些，这次不改登录流程。
- v2.1 里不在这几个 RPC 上的项见下一节。

## v2.1 服务端

对照 v2.1 定稿和当前代码。上面的客户端 RPC 缺口不在这一节里。新行为默认关闭，登录流程没改。

### 这次补上的

`help.getAppConfig` 增加「关于 Tellomi」和个性化推荐要用的键。官方客户端不读这些键。

`TELLOMI_ABOUT_DISCLOSURE` 不是 `1` 时，`tellomi_personalized_recommend_default` 是 false，客户端不显示「个性化推荐」。备案号、模型名、登记编号这三个键不出现。

`TELLOMI_ABOUT_DISCLOSURE=1` 并且 `TELLOMI_ALGORITHM_FILING_NUMBER` 是一段公开编号时：

- `tellomi_personalized_recommend_default` 为 true。这是开关的默认打开（D-58），不是某个用户此刻的选择。客户端在「隐私与安全」里显示开关，默认开。用户关掉之后，推荐流改为不针对个人的排序；排序在 content-engine，本进程不保存这个选择，也不做排序。
- `tellomi_algorithm_filing_number` 是这段编号，给「关于 Tellomi」页（D-59）。

同一个开关下，`TELLOMI_AI_PUBLIC_MODEL`、`TELLOMI_AI_REGISTRATION_NUMBER` 去掉首尾空白后非空，才分别下发 `tellomi_ai_model_name`、`tellomi_ai_registration_number`（D-50）。没填就没有这个键。控制字符、超过 128 个字符、或正文里出现 telegram / t.me 的，当成没填。`true`、`yes` 不能代替 `1`。

配置一变，`help.getAppConfig` 的 hash 就变，客户端会重新拉。没有新的 RPC，没有新表，也不动 Redis。

### 已经在 master 上、这次没改行为

- 小米推送已经在 `pkg/push`。`TELLOMI_XIAOMI_PUSH=1`，并且 AppSecret 和包名都有，才向 token_type 14 发通知栏消息。否则只存 token，不发。见 `docs/push.md`。
- 帮助中心网址：`TELLOMI_HELP_CENTER=1` 才把 `faq_link` 改成 `https://tellomi.cn/help`（D-40）。默认仍是 `https://tellomi.cn/faq`。页面不由本进程提供，这次不改默认值。

### 还留着的 v2.1 服务端

- 帮助网页搬到 Cloudflare Pages（D-62），以及什么时候把 `TELLOMI_HELP_CENTER` 打开。打开之前要确认 `https://tellomi.cn/help` 已经能打开。
- R2 前面的 CDN 域名（D-63）是 Cloudflare 配置。本仓库已有公开媒体和附件的 base URL。
- 发现和 @tellomi 的海外 Cloud Run、Supabase 只读副本、手机离线队列（D-64 到 D-67）不在这个进程里。`@tellomi` 的每日计数还在进程内存里，第二份实例不能共用。群聊上下文还没做。
- 推送（D-68）：APNs 在挂了 `.p8` 时发送。FCM 和华为的 token 只存储，发送端没装。OPPO、vivo、荣耀、魅族没有通道，等确认小米能不能覆盖这些品牌后再接。从成都能不能打到 FCM，这个仓库里没测。
- 监控和告警仍不做（D-69）。聊天主备只写了 `docs/design/chat-standby.md`，没有代码（D-70）。隐私政策正文在官网，不在这里（D-71）。
- 用户关掉个性化推荐之后，发现流立刻改成不针对个人的排序：客户端把选择交给 content-engine。本进程不下发这个人的当前选择。

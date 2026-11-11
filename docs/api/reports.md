# 举报与 @admin 回执

用户举报之后，bff 写一行 `reports`，并从 `@admin`（用户 777000）给举报人发一条普通私聊。正文带工单号 `T{id}`。这条回执不看机审开关，也不看 HTTP 开关。

机审打开时，同一行会被分成 `violation`、`clear` 或 `needs_human`。违规会再通知被举报人一次，正文不写是谁举报的。没有违规只通知举报人。同一目标的多份举报，每个人只收一次结论。

登录和会话路由不经过这里。

## 谁会入库

| 调用 | 来源字段 |
| --- | --- |
| `account.reportPeer` | `account.reportPeer` |
| `account.reportProfilePhoto` | `account.reportProfilePhoto` |
| `messages.report`（两个 constructor） | `messages.report` |
| `messages.reportSpam` | `messages.reportSpam` |
| `messages.reportReaction` | `messages.reportReaction` |
| `messages.reportEncryptedSpam` | `messages.reportEncryptedSpam` |
| `channels.reportSpam` | `channels.reportSpam` |
| `stories.report` | `stories.report` |
| `ephemeral.reportMessage` | `ephemeral.reportMessage` |
| Discover `POST /internal/v1/reports` | `discover.post` 或 `discover.comment` |

`messages.reportReadMetrics`、`messages.reportMusicListen`、`messages.reportMessagesDelivery` 不是举报，不进这张表。

会话必须把 `/mtproto.RPCReports` 指到 `bff.bff`。`teamgramd/etc/session.yaml`、`teamgramd/etc2/session.yaml` 和 `app/interface/session/etc/session.yaml` 都有这条。没这条路由时，客户端拿到的是 `ENTERPRISE_IS_BLOCKED`，表里没有行，也没有回执。

## 启动日志

bff 起来时打一行：

```
report: ready store=mysql notifier=on review=off (TELLOMI_REPORT_REVIEW is unset) http=off (TELLOMI_REPORT_HTTP is not 1)
```

- `store=memory`：MySQL 没连上。行在进程里，重启就没了。回执仍然会发。
- `notifier=missing`：没有接上发消息的客户端。行在，举报人收不到 `@admin`。
- `review=on (follows TELLOMI_AI)`：没单独开举报机审，但 `@tellomi` 的开关和 key 已经开了，新举报会分类。
- `http=on 127.0.0.1:8816 loopback`：Discover 从另一个容器发不过来。

回执成功是 `report: receipt sent id=<n> user=<id>`。失败是 `report: receipt failed`。没装发送端是 `report: receipt not sent`。正文不进日志。

举报人的 RPC 超时不会取消已经入库的回执。

## 环境变量

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_REPORT_REVIEW` | 空 | `1` 分类。`0` 等人审。空着时，若 `TELLOMI_AI=1` 且有 `TELLOMI_AI_API_KEY`，跟随机审 |
| `TELLOMI_AI_API_KEY` | 空 | 机审用的百炼 key。没有 key 就不分类 |
| `TELLOMI_REPORT_HTTP` | 关 | `1` 才监听下面的内部接口 |
| `TELLOMI_REPORT_KEY` | 空 | 标准 base64，解码后至少 32 字节。调用方把它原样放在 `Authorization: Bearer` |
| `TELLOMI_REPORT_ADDR` | `127.0.0.1:8816` | `off` 不监听。另一个容器要改成它能访问的地址，例如 `0.0.0.0:8816` |
| `TELLOMI_REPORT_SNAPSHOT` | 关 | `1` 才把证据传到对象存储的 `reports/` 前缀 |
| `TELLOMI_REPORT_ETA_HOURS` | `24` | 回执里的预计处理时间 |

`docker-compose.yaml` 会把这些变量传进 teamgram 容器。只写在宿主机环境里、不进 compose，进程看不到。

表在 bff 连上 MySQL 时自动创建，语句同 `teamgramd/deploy/sql/migrate-20261105-reports.sql`。

## 部署

1. 跑包含这次改动的 bff。旧二进制只把 `account.reportPeer` 留在进程内存里，不发 `@admin`。
2. 确认 session 配置里有 `"/mtproto.RPCReports": "bff.bff"`，然后重启 session。
3. 确认 bff 日志有 `report: ready store=mysql notifier=on`。`notifier=missing` 或 `store=memory` 先修 MySQL 和 msg 客户端，再让用户重试。
4. 要机审：`TELLOMI_AI=1` 且 `TELLOMI_AI_API_KEY` 已设置（空的 `TELLOMI_REPORT_REVIEW` 会跟着开），或显式 `TELLOMI_REPORT_REVIEW=1`。不要机审就设 `TELLOMI_REPORT_REVIEW=0`。
5. 真机举报一个用户或一条消息。举报人应在 `@admin` 看到工单。日志有 `report: receipt sent`。`reports` 里有一行。

## Discover（content-engine）

本仓听 `POST /internal/v1/reports`。content-engine 需要自己把发现帖或评论的举报转过来。该仓默认不转发（开关名以该仓为准，已知有 `ENGINE_REPORT_FORWARD`）。本 token 读不到那个私有仓，下面是本仓已经实现的契约，对方按这个接即可。

teamgram：

- `TELLOMI_REPORT_HTTP=1`
- `TELLOMI_REPORT_KEY` 为标准 base64，解码后至少 32 字节
- `TELLOMI_REPORT_ADDR` 必须是 content-engine 能访问的地址。默认 `127.0.0.1:8816` 只在本网络命名空间里。同一 Docker 网络用 `0.0.0.0:8816`，不要把这个端口映射到公网

content-engine：

- 打开它的转发开关
- `POST http://<teamgram 在该网络中的地址>:<端口>/internal/v1/reports`
- `Authorization: Bearer <TELLOMI_REPORT_KEY 的原文，标准 base64>`
- `Content-Type: application/json`

```json
{"reporter_user_id":12,"author_id":34,"post_id":"post-9","comment_id":"","reason":"spam","comment":"","lang_code":"zh-hans"}
```

`reason` 只能是 `spam`、`violence`、`pornography`、`child`、`drugs`、`personal`、`other`。评论举报带 `comment_id`。同一举报人、同一目标再传一次返回原来的工单，不再发第二条回执。

`200`

```json
{"status":"created","id":1,"ticket":"T1"}
```

重试时 `status` 是 `duplicate`。太频繁是 `429`，正文 `{"error":"rate_limited","retry_after":N}`。

转发上面这条之前，content-engine 若还没有数字 user id，先解析 `sub`。content-engine#77 会这么做。这条路由不存在（`404`）时，它把举报留在本地，稍后重试。

### 解析 sub

`POST /internal/v1/reports/subjects`

鉴权与上面相同。

```json
{"subs":["<16 字节 sub 的 base64url，无填充>"]}
```

`subs` 1 到 32 个。带 `=`、解出来不是 16 字节、或不是 base64url，整次是 `400`。

`200`

```json
{"users":[{"sub":"<同一串字节的 base64url，无填充>","user_id":34}]}
```

`user_id` 是 teamgram 的 `users.id`。`discover_subject` 里没有的 `sub` 不出现。重复的 `sub` 只返回一次。这个接口不写 `reports`，也不发 `@admin`。

这些行只在 `TELLOMI_DISCOVER_RELATIONS=1` 且关系密钥、MySQL 都可用时写入：用户成功调用 `discover.getEngineCredential` 或 `discover.getAudienceToken` 之后，bff 记下这次调用者的 `sub → user_id`。开关没开，或者这个人还没拿过凭证，`users` 是空数组。表是 `teamgramd/deploy/sql/migrate-20261104-discover-subject.sql`。

没连上 MySQL 是 `503`，响应头 `Retry-After: 5`，正文 `{"error":"unavailable","retry_after":5}`。这次失败不要记成「没有这个 sub」。表还没建时，正向接口仍按「还没有这个 sub」回答，`users` 为空。反查接口不是这样，见下一节。

### 按 users.id 反查 sub

`POST /internal/v1/reports/subjects/by-users`

鉴权、监听与正向接口相同，不新开端口。content-engine 用它把聊天资料页传来的数字用户 id 解析成发现身份。这个接口不写 `reports`，也不发 `@admin`。它不判断访问者是否被屏蔽、是不是未成年人、列表是否对访问者可见；那些仍由 content-engine 按访问者自己做。

```json
{"user_ids":[34,35,36,37]}
```

`user_ids` 为 1 到 32 个正整数。`0`、负数、空数组、非整数或超过 32 个是 `400`，没有 `Retry-After`。重复的 id 只保留第一次出现的顺序。未带合法 `Authorization` 是 `401`，没有 `Retry-After`。只接受 `POST`。

`200` 表示每一个去重后的 id 都分类完成。数组长度与去重后的请求相同，顺序一致。省略某个 id 不表示任何一种结果。

```json
{"users":[
  {"user_id":34,"status":"mapped","sub":"<16 字节 sub 的 base64url，无填充>"},
  {"user_id":35,"status":"empty"},
  {"user_id":36,"status":"not_found"},
  {"user_id":37,"status":"closed"}
]}
```

| status | 含义 | `sub` |
| --- | --- | --- |
| `mapped` | `users` 有这个 id，`deleted = 0` 且 `restricted = 0`，并且 `discover_subject` 有一条 16 字节 `sub` | 必有，base64url，无填充 |
| `empty` | 账号条件同上，并且这次读取确认 `discover_subject` 没有该 `user_id`。这是经授权且确实为空的主页：账号存在、未删除、未受限、从未建立发现身份 | 没有该字段 |
| `not_found` | `users` 没有这个 id。身份不存在。`discover_subject` 里的孤儿行也不返回 | 没有该字段 |
| `closed` | `users.deleted <> 0` 或 `users.restricted <> 0`。已删除或受限。有合法 `sub` 也不返回 | 没有该字段 |

`empty` 只说明 teamgram 确认了账号状态，并且发现身份确实没有建立。它不是「这位访问者可以看列表」。content-engine 在对外返回空列表之前，仍要做目标级隐私判断。`not_found` 和 `closed` 都不是空主页，不能记成「没有主页」，也不能回公开接口的 `200` 空列表。

`mapped` 只说明这个 teamgram 账号对应一个发现身份。该主页在 content-engine 里是否已删除，由 content-engine 自己的资料行决定。

依赖失败是 `503`，不是 `200`，也不能是空的 `users`。范围包括：数据库没连上、`users` 或 `discover_subject` 不存在或不可读、查询或扫描失败、读到的 `sub` 不是 16 字节、同一 `user_id` 有两条不同的 `sub`、同一 id 的 `deleted` / `restricted` 互相矛盾。响应头 `Retry-After: 5`，正文：

```json
{"error":"unavailable","retry_after":5}
```

这次响应里没有用户结果。调用方至少等待 5 秒再试，不得把请求中的任何一个 id 记成 `not_found`、`closed` 或 `empty`，也不得写入否定缓存。

若一份 `200` 缺了请求过的 id、缺了 `status`、`status` 不是上表四值、`mapped` 没有合法 `sub`，或非 `mapped` 带了 `sub`，调用方应把它当成依赖失败，而不是空主页。本服务不会故意返回这种正文。

和正向 `POST /internal/v1/reports/subjects` 不同：那里在 `discover_subject` 还没建时仍可返回空数组。反查不行。缺表和缺库都是 `503`。

可选：发帖前的分类走 `POST /internal/v1/moderation/review`，正文 `{"post_id":"post-9","text":"...","media_keys":["photos/1"]}`，响应 `{"verdict":"violation"}`。`verdict` 是 `violation`、`ok` 或 `uncertain`。机审没开时直接是 `uncertain`，不调用模型，也不写 `reports`。

人审结论是 `POST /internal/v1/reports/{id}/decision`，正文 `{"decision":"violation"}` 或 `{"decision":"clear"}`。`no_violation` 与 `clear` 相同。

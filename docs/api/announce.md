# 上线公告（服务端）

默认关闭。`TELLOMI_ANNOUNCE` 不是 `1`，或者 `TELLOMI_ANNOUNCE_KEY` 解不出来至少 32 字节时，不监听。`messages.sendMessage` 和登录流程不经过这里。

`help.getAppConfig` 的 `tellomi_announce_enabled` 为 true 时，表示运维打开了这个开关。官方客户端不读这个键。

## 谁收到

每一页最多 50 个未删除、不是机器人的账号，从 `@tellomi`（用户 777001，Tellomi AI）的私聊里收到一条普通文字，并走和普通私聊一样的推送。省略 `from` 时就是 777001。`from` 只能是 777000 到 777005。`TELLOMI_ANNOUNCE_FROM` 可以改默认值。

每一条成功的私聊之后暂停 `TELLOMI_ANNOUNCE_GAP_MS` 毫秒（默认 50，0 表示不等）。部署本身不会发公告：`TELLOMI_ANNOUNCE` 不是 `1` 时不监听，监听之后也要有人调用这个接口。

同一条 `event_id` 对同一个用户只写一次。发送失败会放开这一条，下一次用同一个 `cursor` 再试。已经写成功的不会再写。

另外往 `@news`（频道 1073800001，创建者 `@tellomi_official` / 777002）写一条频道消息，当作存档。频道里现在只有创建者，所以用户不是靠订阅频道看到公告，而是靠上面的私聊。频道写入失败时私聊照样发，响应里 `channel` 是 `failed`，下一次请求会再试频道。

777000 到 777005 这些官方号自己收不到（不能登录）。

## 调用

`POST /internal/v1/announce`

默认 `127.0.0.1:8815`。不要映射到公网。

```
Authorization: Bearer <TELLOMI_ANNOUNCE_KEY 的原文，标准 base64>
Content-Type: application/json
```

```json
{"event_id":"tellomi-ai-launch","text":"**Tellomi AI 正式上线**\n\n您好！","markup":"markdown","cursor":""}
```

`markup` 为 `markdown` 时，`**加粗**` 会变成 `messageEntityBold`，星号不进正文。换行和 `•` 项目符号保留。偏移按 UTF-16 计。也可以不传 `markup`，自己给 `entities`（`type` 目前只接受 `bold`）。两者不要同时给。

`dry_run` 为 true 时只返回人数和预览，不写领取记录，也不发私聊、不发频道。

`cursor` 用上一次响应里的字符串。第一次省略或 `"0"`。它是用户 id 的十进制，不用 JSON 数字，避免超过 2^53 时丢精度。

`200`

```json
{"status":"partial","cursor":"10050","sent":50,"channel":"posted","from":777001}
```

试运行：

```json
{"dry_run":true,"status":"dry_run","count":1200,"sent":1200,"from":777001,"preview":"Hi","entities":[{"type":"bold","offset":0,"length":2}],"channel":"skipped","cursor":"0"}
```

| 字段 | 含义 |
| --- | --- |
| `status` | `partial` 这一页满了，用返回的 `cursor` 再调。`done` 后面没有了 |
| `sent` | 这一页新写出去的私聊条数。已经写过的不计 |
| `channel` | `posted` 频道里有了；`failed` 这次没写上，可以重试；`skipped` 进程没有频道发送端 |

满 50 人时即使刚好是最后一页也会是 `partial`。再调一次，`sent` 为 0、`status` 为 `done` 就结束。

| HTTP | `error` | 怎么做 |
| --- | --- | --- |
| 401 | `unauthorized` | 密钥不对 |
| 400 | `bad_request` | 字段不合法。正文 1–1000 个字符，允许换行和制表符 |
| 503 | `unavailable` | 这一页没写完。用同一个 `cursor` 再试 |
| 404 | `not_found` | 路径不对 |

正文、密钥不进日志。

## 环境变量

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_ANNOUNCE` | 关 | `1` 才启用 |
| `TELLOMI_ANNOUNCE_KEY` | 空 | 标准 base64，解码后至少 32 字节 |
| `TELLOMI_ANNOUNCE_ADDR` | `127.0.0.1:8815` | `off` 不监听 |
| `TELLOMI_ANNOUNCE_FROM` | `777001` | 默认发件人，必须是 777000–777005 |
| `TELLOMI_ANNOUNCE_GAP_MS` | `50` | 每条私聊之后的间隔，0–5000 |

表在 `teamgramd/deploy/sql/migrate-20261108-announce.sql`。

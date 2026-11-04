# 发现：系统推送

给 iOS / Android 客户端，以及 content-engine。站内通知页仍然读引擎的 `GET /v1/feed/me/notifications`（用户的 `discover.getEngineCredential` JWT）。本接口只负责系统推送。

2026-11-04 读 https://github.com/tellomi/content-engine 时仓库是私有的，这里的调用方拿不到 `api/notifications.go` 的字段名。引擎在写完一条站内通知后，按下面的 JSON 调用 teamgram。附录 A 里引擎已有的种类（赞、评论、回复、关注、转发）加上 @ 提及，对应 `kind`。

开关默认关闭。`TELLOMI_DISCOVER_PUSH` 不是 `1` 时不监听，聊天推送照旧。

## 引擎 → teamgram

`POST /internal/v1/discover/notify`

默认只听 `127.0.0.1:8813`。不要映射到公网。

```
Authorization: Bearer <TELLOMI_DISCOVER_PUSH_KEY 的原文，标准 base64>
Content-Type: application/json
```

密钥解码后至少 32 字节，和关系接口的密钥不是同一把。

```json
{
  "event_id": "ntf-01",
  "recipient": "<收件人 sub，16 字节 base64url，无填充>",
  "kind": "like",
  "actor_name": "林",
  "post_id": "post-9",
  "comment_id": "",
  "text": "好看"
}
```

| 字段 | 规则 |
| --- | --- |
| `event_id` | 必填，1–128，字符集 `[A-Za-z0-9._:-]`。同一把 id 第二次返回 `duplicate`，不再推 |
| `recipient` | 必填。teamgram 只在用户拿过 `discover.getEngineCredential` 且 `TELLOMI_DISCOVER_RELATIONS=1` 时记过这个 `sub`。没记过返回 `unknown_recipient`，不要重试 |
| `kind` | `like` `comment` `reply` `mention` `follow` `repost` |
| `post_id` | 除 `follow` 外必填，最长 128 |
| `comment_id` | `reply` 必填，最长 128 |
| `actor_name` | 可空，最长 80 个字符。空则标题用「发现」 |
| `text` | 评论或回复的预览，最长 200。其它 kind 可空 |

`sub` 的算法和 `docs/discover-engine-auth.md` 相同。请求里不要出现 teamgram 的 user id。

### 响应

`200`

```json
{"status": "sent"}
```

| `status` | 含义 |
| --- | --- |
| `sent` | 已交给 APNs / FCM / 华为里这个用户已注册的通道 |
| `muted` | 用户关掉了发现通知。不要重试 |
| `no_devices` | 没有可推的设备。不要重试 |
| `unknown_recipient` | 还没有这个 `sub`。用户下次取凭证后的新事件才能推 |
| `duplicate` | 这把 `event_id` 已经处理过 |

| HTTP | `error` | 引擎怎么做 |
| --- | --- | --- |
| 401 | `unauthorized` | 密钥不对 |
| 400 | `bad_request` | 字段不合法。不要重试同一份 body |
| 503 | `unavailable` | 暂时失败。可以换不换 `event_id` 都行，没成功的 id 会放开 |
| 404 | `not_found` | 路径不对 |

推送失败（对方通道返回错误）仍是 `sent`：和聊天推送一样记日志，不让引擎无限重放。

## 客户端：关掉发现通知

用现成的 `account.updateNotifySettings`，不要新造 TL。

对象是官方号 **@faxian**（用户 id `777005`，另一个用户名 `@discover`）。`peer` 用 `inputNotifyPeer`，里面是这个用户的 `inputPeerUser`。

| 设置 | 效果 |
| --- | --- |
| 没有这一行 | 开。会推 |
| `mute_until` 大于现在 | 关。赞、评论、回复、提及、关注、转发都不推 |
| `show_previews = false` | 仍推，但评论和回复的正文不进横幅 |
| `silent = true` | 横幅没有声音 |

这就是「通知与声音」里发现通知的开关。只静音「所有私聊」不会关掉发现。发现里的 @ 提及也不会绕过这个开关。

`mute_until = 0` 再打开。

## 客户端：系统推送里有什么

和聊天推送同一条设备登记（`account.registerDevice`）。会发出的通道：APNs（`token_type` 1，要有 `.p8`）、FCM（2，要装发送端）、华为（13，要装发送端）、小米（14，`TELLOMI_XIAOMI_PUSH=1`）。没装的通道不计入 `sent`。一台都没有时是 `no_devices`。

横幅标题是 `actor_name`。正文：

| `kind` | `loc_key` | 简体正文 |
| --- | --- | --- |
| `like` | `DISCOVER_LIKE` | 赞了你的帖子 |
| `comment`，无预览或没有正文 | `DISCOVER_COMMENT` | 评论了你的帖子 |
| `comment`，有正文 | `DISCOVER_COMMENT_TEXT` | 评论了你的帖子：{正文} |
| `reply` | `DISCOVER_REPLY` / `DISCOVER_REPLY_TEXT` | 回复了你的评论 |
| `mention` | `DISCOVER_MENTION` | 在帖子里提到了你 |
| `follow` | `DISCOVER_FOLLOW` | 关注了你 |
| `repost` | `DISCOVER_REPOST` | 转发了你的帖子 |

繁体和英文由设备语言决定（和聊天推送同一套 `lang_code` / `system_lang_code`）。

点开帖子看这些字段，不要拿 `from_id` 去开聊天。`from_id` 固定是 `777005`，给还不认识发现推送的旧客户端一个无害的官方号。

解密后的 `p`（Android 的明文，iOS 通知扩展解密后的 JSON）里有：

```json
{
  "discover": "1",
  "kind": "comment",
  "post_id": "post-9",
  "comment_id": "",
  "from_id": "777005",
  "msg_id": "0"
}
```

Android 的 `custom` 是对象，里面是同样的键。FCM 数据里的 `custom` 是这个对象的 JSON 字符串。`loc_key` 用下划线，和聊天推送一样。

iOS 解密后的对象**没有** `loc-key`（有的话通知扩展会把它当成服务事件，不显示横幅）。`discover`、`kind`、`post_id`、`comment_id` 是顶层字符串。通知扩展要抄进通知的 userInfo，App 才能打开 `post_id`。

`comment_id` 空则省略。`follow` 可以没有 `post_id`。

## 环境变量

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_DISCOVER_PUSH` | 关 | `1` 才监听 |
| `TELLOMI_DISCOVER_PUSH_KEY` | 空 | 标准 base64，解码后至少 32 字节 |
| `TELLOMI_DISCOVER_PUSH_ADDR` | `127.0.0.1:8813` | `off` 不监听 |

还要开 `TELLOMI_DISCOVER=1` 和 `TELLOMI_DISCOVER_RELATIONS=1`，否则没有 `sub → user id`，推送会一直是 `unknown_recipient`。表 `discover_push_event` 在 `teamgramd/deploy/sql/migrate-20261106-discover-push.sql`。

监听跑在 `sync` 进程里，因为设备表和推送都在那里。

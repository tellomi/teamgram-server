# @tellomi（服务端，一期）

用户 id `777001`，用户名 `@tellomi`。默认关闭。`TELLOMI_AI` 不是 `1`，或者没有 `TELLOMI_AI_API_KEY` 时，私聊和下面的内部接口都不工作，`messages.sendMessage` 跟以前一样。

模型走阿里云百炼的 OpenAI 兼容接口。主模型 `qwen-plus`，月预算到 80% 换成 `qwen-turbo`，百炼失败或超时再换 `deepseek-v3`（仍是百炼上的模型名，这样内容安全还在）。内容安全拦下时不换模型，回复固定的「这个问题我暂时不能回答」。

每次模型回复末尾单独一行是 `AI 生成`，并且这一行是 `messageEntityBold`。限额提示不加这行，也不是模型生成的。

## 客户端：私聊

不新增 TL。用户给 `@tellomi` 发普通 `messages.sendMessage`。服务端在消息已经写入之后，在后台回复。开关关闭时没有这步。

定时消息不触发回复。群聊不在这一期。

`help.getAppConfig` 的 `tellomi_ai_enabled` 为 true 时，客户端可以显示入口。官方客户端不读这个键。`tellomi_ai_compose_enabled` 仍是 false。输入栏的 `messages.composeRichMessageWithAI` 不调用百炼：校对、加表情和语气把原文还回去，`translate_to_lang` 用和 `messages.translateText` 一样的本地词表。

| 情况 | 用户看到的文字 |
| --- | --- |
| 正常 | 模型文字，然后空行，然后 `AI 生成` |
| 百炼内容安全（`data_inspection_failed` / `DataInspectionFailed`，以及回复侧的 `ResponseInspectionFailed`） | 这个问题我暂时不能回答 |
| 当天第 101 次 | 今天的提问次数已用完 |
| 一分钟内第 6 次 | 操作太频繁，请稍后再试 |
| 月预算到 100% | 本月的提问额度已用完 |
| 主模型和 DeepSeek 都超时 | 不发第二条消息。用户自己的那条已经发出 |

同一条 `random_id` 只回答一次。限额按进程计，重启后重新算。回复另外写入 `tellomi_ai_reply`（提示、回复、模型、是否被内容安全拦住）。密钥不进这张表，也不进日志。

## 引擎：发现里的 @tellomi

引擎解析到 @tellomi 之后调用：

`POST /internal/v1/ai/complete`

默认 `127.0.0.1:8814`。不要映射到公网。

```
Authorization: Bearer <TELLOMI_AI_INTERNAL_KEY 的原文，标准 base64>
Content-Type: application/json
```

```json
{
  "event_id": "mention-1",
  "user_sub": "<提问人 sub，16 字节 base64url，无填充>",
  "text": "用户的问题"
}
```

`sub` 必须已经在 `discover_subject` 里（用户拿过 `discover.getEngineCredential`，并且开了 `TELLOMI_DISCOVER_RELATIONS`）。

`200`

```json
{
  "text": "回答\n\nAI 生成",
  "model": "qwen-plus",
  "ai_generated": true,
  "refused": false
}
```

引擎把 `text` 写成官方账号的评论，带 `ai_generated`，不走帖子机审（D-52）。`refused: true` 仍要写上，这是内容安全的固定句，也带 `AI 生成`。`limited` 为 `daily`、`minute` 或 `budget` 时 `ai_generated` 是 false，把 `text` 写出来给用户看。

同一把 `event_id` 再调一次会拿回同一段文字，不会再次计次、也不会再调模型。

| HTTP | `error` | 含义 |
| --- | --- | --- |
| 401 | `unauthorized` | 密钥不对 |
| 400 | `bad_request` | 字段不合法，或 `sub` 格式不对 |
| 504 | `timeout` | 两次模型调用都超时。可以重试同一把 `event_id` |
| 502 | `model` | 模型返回了别的错误 |
| 503 | `unavailable` | 暂时不能处理 |

`200` 且 `{"status":"unknown_user"}`：还没有这个 `sub`。不要重试，等用户下次取凭证。

## 环境变量

| 名字 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_AI` | 关 | `1` 才启用 |
| `TELLOMI_AI_API_KEY` | 空 | 百炼 API key。不写日志 |
| `TELLOMI_AI_BASE_URL` | `https://dashscope.aliyuncs.com/compatible-mode/v1` | 兼容模式。海外要换国际站时改这里 |
| `TELLOMI_AI_MODEL_PRIMARY` | `qwen-plus` | 主模型 |
| `TELLOMI_AI_MODEL_SMALL` | `qwen-turbo` | 预算 80% 之后 |
| `TELLOMI_AI_MODEL_FALLBACK` | `deepseek-v3` | 失败或超时的备用，仍走百炼 |
| `TELLOMI_AI_TIMEOUT_SECONDS` | `12` | 单次调用上限。备用再给同样长的时间 |
| `TELLOMI_AI_MONTHLY_BUDGET` | 空 | 月预算。空表示不封顶、也不换小模型 |
| `TELLOMI_AI_PRICE_PER_1K` | 空 | 每 1000 token 的价钱，用来算预算。空则不计花费 |
| `TELLOMI_AI_INTERNAL_KEY` | 空 | 引擎调用的密钥。缺了只关这个 HTTP，私聊仍可用 |
| `TELLOMI_AI_ADDR` | `127.0.0.1:8814` | `off` 不监听 |

表 `tellomi_ai_reply` 在 `teamgramd/deploy/sql/migrate-20261106-tellomi-ai.sql`。

发现的系统推送不在这里，见 `docs/api/discover-ai.md`。

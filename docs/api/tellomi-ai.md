# @tellomi（服务端，一期）

用户 id `777001`，用户名 `@tellomi`。默认关闭。`TELLOMI_AI` 不是 `1`，或者没有 `TELLOMI_AI_API_KEY` 时，不调用模型，内部接口也不监听。私聊和群里点到这个账号时，回复固定的「AI 助手暂未开放，敬请期待。」`messages.sendMessage` 本身的返回不变。

模型走阿里云百炼的 OpenAI 兼容接口。主模型 `qwen-plus`，月预算到 80% 换成 `qwen-turbo`，百炼失败或超时再换 `deepseek-v3`（仍是百炼上的模型名，这样内容安全还在）。内容安全拦下时不换模型，回复固定的「这个问题我暂时无法回答，换个话题试试吧。」上游返回 `content_filter` / `content filter` 也走这一句。

助手回复（私聊、群、以及下面的 complete 接口）不再在末尾加 `AI 生成`，也不再给这一行加 `messageEntityBold`。账号本身已经标明是 AI。限额提示同样不加这行，也不是模型生成的。举报审核发给管理员的通知如果原来带这一行，仍保留。

## 客户端：私聊

不新增 TL。用户给 `@tellomi` 发普通 `messages.sendMessage`，或 `messages.sendMedia` / `messages.sendMultiMedia`。服务端在消息已经写入之后，在后台回复。没有文字的贴纸、图片和其它媒体不调用模型，回复「目前我只能处理文字消息哦。」带了文字说明的媒体按这段文字处理。

定时消息不触发回复。

## 谁可以用

只有允许名单里的用户会调用模型。`TELLOMI_AI_ALLOW_USERNAMES` 是逗号分隔的用户名，不带 `@`，忽略大小写。没设置或只有空白时，默认是 `erel,lfnuebfci,pikaqiu`。整段设成 `*` 时所有人都可以用。`TELLOMI_AI_ALLOW_USER_IDS` 是可选的用户 id，逗号分隔，这些 id 不查用户名也允许。

用户名通过现有的 `user.getListByUsernameList` 解析成用户 id，成功的结果缓存一分钟。查用户名失败不调用模型，回复「AI 暂时有点忙，请稍后再试。」失败结果缓存 15 秒。频道占用的同名用户名不算用户。

不在名单里的人发私聊、在群里 @、或回复 777001 时，不调用模型，回复「AI 助手目前仅对部分用户开放。如需使用 AI，请先完成实名认证。」私聊里同一个人一分钟最多发一次这句。群里同一个人在同一个群十分钟最多一次。

`tellomiai.RealNameChecker` 和 `SetRealNameChecker` 是以后换成实名校验的钩子。装上之后不再看允许名单：`Verified` 为真才能用模型，为假走上面这句。现在没有安装这个钩子。

## 客户端：群

普通群和超级群里，只有这两种消息会回复：

- 用户名 mention `@tellomi`，或指向用户 777001 的 mention name（含 `inputMessageEntityMentionName`）
- 回复的是 777001 自己发出的那一条

客户端一条实体都没带时，正文里单独的 `@tellomi` 也算用户名 mention（服务端本来就会把它存成 mention）。带了别的实体、但没有上述 mention 的，不回复。

频道（广播）不回复。777001 自己的消息不回复。机器人发的不回复。

回复写进同一个群，`reply_to` 指向触发的那一条。模型只看到两段文字：触发消息去掉 `@tellomi` 之后的正文；如果这条是回复，再加上被回复那一条的正文。不读群里的其它历史。去掉 mention 之后没有可问的文字，或者这条是没有说明的贴纸或其它媒体，不调用模型，回复「目前我只能处理文字消息哦。」贴纸即使回复的是 777001，也不把被回复的正文拿去问模型。

普通群里，触发消息在 777001 自己收件箱里的 id 要等异步扇出写完，`sendMessage` 返回时经常还没有。这一次 `message.getPeerUserMessage` 失败不中断：mention、是不是在回复 777001、成员资格和模型调用照常进行，`reply_to` 先记为 0。模型返回后每隔 200ms 再查，最多约 3 秒。查到就用 777001 自己的消息 id；仍然没有就发出回复且不带 `reply_to`，不把回复丢掉。失败时打一行 `tellomi ai: group box id pending chat=<id>`，日志里没有正文。超级群的消息 id 对所有成员相同，不查这张表，也不等待。

777001 必须已经是这个群的成员才能回复。`msg.sendMessageV2` 没有系统账号旁路：普通群里发送者没有参与者记录时返回 `CHAT_WRITE_FORBIDDEN`，超级群里不是成员时返回 `CHANNEL_PRIVATE`。这里先做同样的检查，不是成员就不调用模型。已退出或群已解散也不回复。频道即使 777001 是管理员也不回复。

限额和私聊共用：每人每分钟 5 条，每天 100 条。另外每个群每分钟 20 条。数字没变。满了都回复固定的句子，不调用模型。已经超过当天 100 条的人，一小时最多收到一次「次数用完」。一分钟内第 6 条及之后，这一分钟只提醒一次。一个群超过 20 条时，这一分钟只在这个群提醒一次，避免刷屏。月预算用完也是一小时最多一次。

`help.getAppConfig` 的 `tellomi_ai_enabled` 为 true 时，客户端可以显示入口。官方客户端不读这个键。

`messages.translateText` 和 `messages.translateRichMessage` 另走 `TELLOMI_TRANSLATE=1`，并且同样要有 `TELLOMI_AI_API_KEY`。`tellomi_translate_enabled` 只在这两样都有时为 true。关掉或模型失败是 `500 TRANSLATE_REQ_FAILED`，不返回原文冒充译文。

`messages.composeRichMessageWithAI` 走 `TELLOMI_AI_COMPOSE=1` 加上同一把 key。`tellomi_ai_compose_enabled` 只在这两样都有时为 true。校对、加表情、语气和 `translate_to_lang` 都调用百炼（`qwen-plus`，失败再换 `deepseek-v3`）。关掉或模型失败是 `500 AICOMPOSE_TIMEOUT`。不返回 `AICOMPOSE_FLOOD_PREMIUM`。这两个开关都不依赖 `TELLOMI_AI`。

| 情况 | 用户看到的文字 | 环境变量 |
| --- | --- | --- |
| 正常 | 模型文字，末尾没有 `AI 生成` | |
| 不在允许名单，或实名钩子返回未认证 | AI 助手目前仅对部分用户开放。如需使用 AI，请先完成实名认证。 | `TELLOMI_AI_MSG_NOT_ALLOWED` |
| 百炼内容安全（`data_inspection_failed` / `DataInspectionFailed` / `content_filter`，以及回复侧的 `ResponseInspectionFailed`） | 这个问题我暂时无法回答，换个话题试试吧。 | `TELLOMI_AI_MSG_BLOCKED` |
| 当天第 101 次 | 今天的 AI 使用次数已用完，明天再来找我吧～ | `TELLOMI_AI_MSG_DAILY` |
| 一分钟内第 6 次 | 消息有点太快了，请稍等一分钟再试。 | `TELLOMI_AI_MSG_MINUTE` |
| 同一个群一分钟第 21 次 | 本群 AI 使用太频繁啦，请稍后再试。 | `TELLOMI_AI_MSG_CHAT` |
| 月预算到 100% | 本月的提问额度已用完 | `TELLOMI_AI_MSG_BUDGET` |
| 模型失败、超时、用户名服务暂时失败、或没有配模型客户端 | AI 暂时有点忙，请稍后再试。 | `TELLOMI_AI_MSG_BUSY` |
| 空消息、只有 `@tellomi`、贴纸或没有文字的媒体 | 目前我只能处理文字消息哦。 | `TELLOMI_AI_MSG_TEXT_ONLY` |
| `TELLOMI_AI` 没开，但有人找这个账号 | AI 助手暂未开放，敬请期待。 | `TELLOMI_AI_MSG_DISABLED` |

上面除了「正常」一行，都不调用模型来写这句话。内容安全被拦住时，主模型的这一次调用已经发生，但不会再换备用模型。别的模型错误会先试一次备用模型，两次都失败才发「有点忙」。同一条 `random_id` 还在处理时不另发一句。用户名解析失败用「有点忙」，不用「未开放」或「请先实名」，避免把故障说成资格问题。

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
  "text": "回答",
  "model": "qwen-plus",
  "ai_generated": true,
  "refused": false
}
```

引擎把 `text` 写成官方账号的评论，带 `ai_generated`，不走帖子机审（D-52）。`refused: true` 仍要写上，这是内容安全的固定句，正文就是「这个问题我暂时无法回答，换个话题试试吧。」，不再加 `AI 生成`。`limited` 为 `daily`、`minute`、`budget`、`allow` 或 `error` 时 `ai_generated` 是 false，把 `text` 写出来给用户看。`allow` 是不在允许名单里。`error` 是用户名服务暂时失败。空的 `text` 仍然是 400 `bad_request`，不会变成文字提示。群里的每分钟 20 条上限不会出现在这个接口。模型超时和模型错误仍然是下面的 504 / 502，聊天里的用户则会看到「AI 暂时有点忙，请稍后再试。」同一把 `event_id` 可以重试。

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
| `TELLOMI_AI_ALLOW_USERNAMES` | `erel,lfnuebfci,pikaqiu` | 逗号分隔，不带 `@`，忽略大小写。`*` 表示所有人。空白等同于没设置 |
| `TELLOMI_AI_ALLOW_USER_IDS` | 空 | 可选。逗号分隔的用户 id，不查用户名也允许 |
| `TELLOMI_AI_MSG_NOT_ALLOWED` | AI 助手目前仅对部分用户开放。如需使用 AI，请先完成实名认证。 | 不在名单里 |
| `TELLOMI_AI_MSG_DAILY` | 今天的 AI 使用次数已用完，明天再来找我吧～ | 当天额度用完 |
| `TELLOMI_AI_MSG_MINUTE` | 消息有点太快了，请稍等一分钟再试。 | 一分钟太频繁 |
| `TELLOMI_AI_MSG_CHAT` | 本群 AI 使用太频繁啦，请稍后再试。 | 一个群一分钟太频繁 |
| `TELLOMI_AI_MSG_BUSY` | AI 暂时有点忙，请稍后再试。 | 模型、超时或用户名服务失败 |
| `TELLOMI_AI_MSG_BLOCKED` | 这个问题我暂时无法回答，换个话题试试吧。 | 上游内容安全 |
| `TELLOMI_AI_MSG_TEXT_ONLY` | 目前我只能处理文字消息哦。 | 空消息或非文字 |
| `TELLOMI_AI_MSG_DISABLED` | AI 助手暂未开放，敬请期待。 | 总开关关着 |
| `TELLOMI_AI_MSG_BUDGET` | 本月的提问额度已用完 | 月预算用完 |
| `TELLOMI_TRANSLATE` | 关 | `1` 才启用 `messages.translateText` / `messages.translateRichMessage`。还要有 API key |
| `TELLOMI_AI_COMPOSE` | 关 | `1` 才启用 `messages.composeRichMessageWithAI`。还要有 API key。`tellomi_ai_compose_enabled` 跟着它 |

表 `tellomi_ai_reply` 在 `teamgramd/deploy/sql/migrate-20261106-tellomi-ai.sql`。

发现的系统推送不在这里，见 `docs/api/discover-ai.md`。

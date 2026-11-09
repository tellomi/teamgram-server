# 处罚复查契约

本文件给 content-engine 和客户端。服务端实现在 `pkg/enforcement`。新建处罚默认关闭。未确认的 1/3/7/30 天档位、申诉 SLA、7696 词库都不在这里生效。

## 认证

进程只在 `TELLOMI_ENFORCEMENT_STORE=mysql` 且 `TELLOMI_ENFORCEMENT_MYSQL` 指向**聊天库以外**的库时安装。库名 `teamgram` 会被拒绝。`memory` 和空的 `TELLOMI_ENFORCEMENT_STORE` 不会安装。`TELLOMI_ENFORCEMENT_CREATE=1` 才允许新建案件。`TELLOMI_ENFORCEMENT_REVIEW=1` 才做举报确认和内容判定。

申诉模型只在 `TELLOMI_AI=1` 且 `TELLOMI_AI_API_KEY` 已设置时使用现有的 `pkg/tellomiai.Client`。没有这两项时，申诉停在 `pending_review`，不调用模型。

## 状态

| 状态 | 普通会话 | 读 | 写 | 发现读 | 发现写 |
| --- | --- | --- | --- | --- | --- |
| 无记录 | 允许 | 允许 | 允许 | 允许 | 允许 |
| `scoped_restriction` 且范围含 `discover_post` | 允许 | 允许 | 只拒绝该范围 | 允许浏览 | 拒绝发帖 |
| `read_only_freeze` | 允许 | 允许 | 拒绝 | 允许 | 拒绝 |
| `permanent_disabled` | 不签发 | 拒绝 | 拒绝 | 拒绝 | 拒绝 |
| `security_lock` | 不签发普通会话 | 聊天阅读按安全恢复，不在本复查里放行聊天 | 拒绝 | 允许浏览 | 拒绝发帖和新凭证 |

`discover_post` 不禁止发现浏览。这和 PRD TC-18 一致。上一版契约把「发现读」写成也会被这个范围拒绝，那个句子作废。`security_lock` 不是处罚记录；复查只在写和新凭证上返回 `ACCOUNT_SECURITY_LOCK`。没有锁记录时不会报这个状态。

永久停用后的身份证明是处置凭证，不是普通 auth key，也不是发现 JWT。凭证只能读 777000 通知和提交自己的申诉。默认 15 分钟，过期后要重新核验。

时间以 UTC 存储。通知正文里的钟点是北京时间（UTC+8）。

## 错误码

| 码 | 何时 |
| --- | --- |
| `IDENTIFIER_RESTRICTED` | 这一个手机号或邮箱不能登录，账号本身不一定被处罚 |
| `ACCOUNT_SCOPED_RESTRICTION` | 这一项操作在限制范围内 |
| `ACCOUNT_READ_ONLY` | 只读冻结下的写操作 |
| `ACCOUNT_PERMANENT_DISABLED` | 永久停用。不发普通会话，已有会话的下一次受保护请求也返回它 |
| `ACCOUNT_SECURITY_LOCK` | 安全锁，不是处罚 |

`PHONE_NUMBER_BANNED` 不是账号处罚。

## 发现引擎：已发出的 JWT

本进程签发新凭证时，永久停用直接拒绝。已经发出、尚未过期的 JWT 不在本进程里保存明文。引擎在每次读和写之前调用 HTTP，不是进程内函数。

`POST /internal/v1/enforcement/credentials/check`

处理函数是 `enforcement.CredentialHandler`。本进程默认不监听。`TELLOMI_ENFORCEMENT_ADDR=0.0.0.0:8820` 时，在容器网络里同时提供本接口和下面的内容接口。宿主机 compose 不映射 8820。`ListenLoopback` 仍只接受 `127.0.0.1` 或 `::1`，隔离演练用它。`ServeNetwork` 只接受 `0.0.0.0`、`::`、`127.0.0.1`、`::1`，拒绝写成某个公网 IP。处理函数不会根据请求去访问任何 URL。

content-engine 使用：

```
ENGINE_ENFORCEMENT_URL=http://teamgram:8820
ENGINE_ENFORCEMENT_KEY=<与 TELLOMI_ENFORCEMENT_SERVICE_KEY 相同的标准 base64>
```

`ENGINE_ENFORCEMENT_URL` 是 origin，没有路径。请求是 `POST http://teamgram:8820/internal/v1/enforcement/credentials/check`，`Authorization: Bearer <那串 base64>`。

本仓的 GitHub 令牌读不到 content-engine 仓库，因此不能在这里核对引擎是否拒绝非 loopback 的 `http` origin。如果引擎只接受 `https` 或 `127.0.0.1`，它需要放行 compose 网络上的 `http://teamgram:8820`。不要把 8820 发布到宿主机；若引擎不能放行明文，应在引擎一侧加内部 TLS，而不是改成公网端口。

`CheckCredential(userID, write bool, now)` 只在本进程内部使用：HTTP 层已经从凭证解出用户号之后才调用它。引擎不传 `userID`，也不调用这个函数。

请求：

```http
POST /internal/v1/enforcement/credentials/check
Authorization: Bearer <标准 base64(服务密钥)>
```

```json
{"credential": "<discover JWT>", "write": true}
```

服务密钥只从配置 `TELLOMI_ENFORCEMENT_SERVICE_KEY` 读取。值是至少 32 字节的标准 base64，和 `discoverpush`、`announce`、`discoverrel` 的服务间密钥同一比较方式：请求头里的 Bearer 先按标准 base64 解码，再与配置中的原始字节做常量时间比较。未配置、请求没带这个头、密钥错误、或不是 `Bearer`，都返回 HTTP 401，响应体相同，`allow: false`，`status` 为 `unauthorized`。响应不区分是哪一种。这个 Bearer 只证明调用方身份，不能指定用户。`f54863fc` 还没有这道校验；content-engine `9352515` 对那个头的联调因此不会带上它。

进入处理函数时，若请求上下文的截止时间已经过去，返回 HTTP 504，`status` 为 `unavailable`，`allow: false`。处理函数不另外计时。验签失败和存储失败仍是 HTTP 503 `unavailable`。

服务端用现有 `discoverauth.Verify` 核对 HS256。`sub` 不是用户号，也不能把看起来像数字的 `sub` 当成用户号。它是 `base64url(HMAC-SHA256(pseudonymKey, 十进制 user id) 的前 16 字节)`，新旧发现 JWT 都是这个算法。

用户号只来自服务端自己写入的 `credential_subject`。签发 `discover.getEngineCredential` 或 `discover.getAudienceToken` 时，若处罚服务已安装，就用同一把 pseudonym key 把这个 `sub` 记下来。已经发出的旧票没有单独的 uid 字段；只要该用户被用同一把 key 记过一次，旧票的 `sub` 就能对上。这不是猜测。没有这行记录时返回 HTTP 403，`status` 和 `code` 都是 `enforcement_unavailable`，`allow: false`。不编用户号。这和 content-engine `1347091e256a1de1a971ed7685aa7154e72568ad` 的读写都拒绝一致。显式补记是 `RecordSubject`，不在进程启动时扫描用户表。请求头 `X-User-Id` 同样忽略。

请求里的 `user_id` 被忽略。服务 Bearer 不能指定用户。

| `status` | `allow` | 含义 |
| --- | --- | --- |
| `clear` | true | 已安装，读成功，这次操作没有被处罚或锁挡住 |
| `enforced` | false | 已安装，读成功，按上表拒绝 |
| `enforcement_unavailable` | false | HTTP 403。票验签通过但没有服务端 sub 绑定，或可信身份不足。读写都拒绝。不是处罚记录 |
| `unavailable` | false | HTTP 503。验签失败或存储失败。不是「没有处罚」。只有请求上下文在进入处理函数时已经过期，才返回 HTTP 504，状态仍是 `unavailable` |
| `disabled` | false | HTTP 503。处理器或处罚服务没安装。不是成功放行 |
| `unauthorized` | false | HTTP 401。服务密钥未配置、缺失或错误。三种情况的响应体相同 |

没有「允许」缓存。`401`、`403`、`5xx` 和无法解析的决定都不是放行。读请求如果超时或收到 5xx，不得把已经见到的 `permanent_disabled` 或 `security_lock` 清掉。写请求遇到超时或 5xx 直接拒绝。

处罚词与 content-engine `1347091e256a1de1a971ed7685aa7154e72568ad` 一致。该引擎 `9352515` 对服务端 `f54863fc` 做了本机 `127.0.0.1` 联调。服务密钥这道校验新于那个头，所以那次联调没有覆盖 401。`6710d1a3` 不能代表当前实现。

```json
{"allow": false, "status": "enforced", "code": "ACCOUNT_PERMANENT_DISABLED", "state": "permanent_disabled"}
```

`allow: false` 时引擎必须拒绝这次读或写，不能因为 JWT 的 `exp` 还没到就放行。读用 `"write": false`。本仓测试覆盖这个判定。引擎进程这一批没有改，由引擎线按本文件接入。

失败策略：复查调用失败、超时或未知状态时，读和写都拒绝。不要把未知状态缓存成允许，也不要把它写成一条新的永久处罚。拿到 `ACCOUNT_PERMANENT_DISABLED` 后，读写都拒绝。

## 内容判定

content-engine 自己调用。本仓不改引擎。账号封禁不走这里。

`POST /internal/v1/enforcement/content/review`

鉴权与凭证复查相同。`TELLOMI_ENFORCEMENT_REVIEW` 不是 `1`，或模型不可用时，返回 HTTP 503，`status` 为 `unavailable` 或 `disabled`。不要把 503 当成下架或保留。

```json
{"post_id":"post-9","comment_id":"","author_user_id":34,"text":"正文","report_count":1}
```

`post_id` 和 `comment_id` 至少有一个。

`200`

```json
{"case_id":"C0123456789abcdef","decision":"remove","confidence":0.96,"reason":"理由"}
```

`decision` 是 `remove`、`keep` 或 `pending_admin`。置信度不低于 `TELLOMI_ENFORCEMENT_CONFIDENCE`（默认 0.8）且模型给出 `remove` 或 `keep` 时，当场返回，不打扰管理员。低于阈值，或模型给出 `uncertain`，`decision` 是 `pending_admin`，并按账号确认的同一方式私聊管理员。

管理员回复之后，引擎再查：

`POST /internal/v1/enforcement/content/decision`

```json
{"case_id":"C0123456789abcdef"}
```

`200`

```json
{"case_id":"C0123456789abcdef","decision":"remove","confidence":0.4,"reason":"理由","admin":"erel","answer":"是"}
```

`pending_admin` 表示还没回复。`remove` 是下架，`keep` 是保留。没有这个案件是 HTTP 404。密钥不对是 HTTP 401，响应体与凭证复查的 401 相同。

## 处置凭证路由

永久停用的登录不绑定普通会话。服务端把处置凭证暂存在这次已核验的 `phone_code_hash`（或未来登录令牌）上，只能领取一次。

`POST /internal/v1/enforcement/disposition/claim`

```json
{"proof": "<phone_code_hash>"}
```

```json
{"token": "<grant>", "expires": "2026-11-09T03:15:00Z"}
```

之后：

`POST /internal/v1/enforcement/disposition/notices`  
`POST /internal/v1/enforcement/disposition/appeals`

`Authorization: Disposition <grant>`

申诉正文只有 `sanction_id` 和 `text`。用户号来自凭证记录。用这张凭证申诉别人的案件会被拒绝。这两条路由不能发消息；未知路径是 404。默认不监听，挂载规则与上面的复查接口相同。

## 客户端

当前客户端保持官方 Telegram 行为。本文件不要求客户端接入自定义处置页。服务端仍对受保护操作返回上表错误码。处置 HTTP 只给已经持有一次性 proof 或处置凭证的调用方，不从客户端自报的用户号取数。

## 通知投递

处罚行和 outbox 行在同一次数据库事务里提交。事务失败则两边都没有。事务成功之后才调用 777000 的 `MsgPushUserMessage`。

因此：

- 库提交成功、投递失败：处罚有效，outbox 状态是 `retry`，进程重启后 `Flush` 再投。
- 对方已收下，但状态更新没写上：下次会再投一次。这是至少一次，不是外部 exactly-once。
- 幂等键是 `案件编号:事件`。已标记 `sent` 的键不会再投。

通知发送者固定是用户 777000，不看用户名是否叫 admin。

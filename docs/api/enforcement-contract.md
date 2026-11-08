# 处罚复查契约

本文件给 content-engine 和客户端。服务端实现在 `pkg/enforcement`。新建处罚默认关闭。未确认的 1/3/7/30 天档位、申诉 SLA、7696 词库都不在这里生效。

## 认证

进程只在 `TELLOMI_ENFORCEMENT_MYSQL` 指向**聊天库以外**的库时安装。库名 `teamgram` 会被拒绝。`TELLOMI_ENFORCEMENT_STORE=memory` 不会安装。`TELLOMI_ENFORCEMENT_CREATE=1` 才允许新建案件。

申诉模型只在 `TELLOMI_AI=1` 且 `TELLOMI_AI_API_KEY` 已设置时使用现有的 `pkg/tellomiai.Client`。没有这两项时，申诉停在 `pending_review`，不调用模型。

## 状态

| 状态 | 普通会话 | 读 | 写 | 发现读 | 发现写 |
| --- | --- | --- | --- | --- | --- |
| 无记录 | 允许 | 允许 | 允许 | 允许 | 允许 |
| `scoped_restriction` | 允许 | 允许 | 只拒绝范围 | 允许，除非范围含 `discover_post` | 范围含 `discover_post` 时拒绝 |
| `read_only_freeze` | 允许 | 允许 | 拒绝 | 允许 | 拒绝 |
| `permanent_disabled` | 不签发 | 拒绝 | 拒绝 | 拒绝 | 拒绝 |
| `security_lock` | 不签发 | 拒绝 | 拒绝 | 拒绝 | 拒绝 |

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

本进程签发新凭证时，永久停用直接拒绝。已经发出、尚未过期的 JWT 不在本进程里保存明文。引擎在每次读和写之前调用：

`CheckCredential(userID, write bool, now)`

约定的 HTTP 形状（引擎来拉，不是本进程主动推）：

`POST /internal/v1/enforcement/credentials/check`

处理函数是 `enforcement.CredentialHandler`。本进程默认不监听。调用方自己挂到隔离地址，并设置非空 Bearer。空密钥会拒绝全部请求。

```json
{"user_id": 42, "write": true}
```

```json
{"allow": false, "code": "ACCOUNT_PERMANENT_DISABLED", "state": "permanent_disabled"}
```

`allow: false` 时引擎必须拒绝这次读或写，不能因为 JWT 的 `exp` 还没到就放行。读用 `"write": false`。本仓测试覆盖这个判定。引擎进程本身这一批没有改。

失败策略：复查调用失败时，写操作拒绝；读操作在引擎尚未拿到该用户的停用结论前不要清空本地聊天。拿到 `ACCOUNT_PERMANENT_DISABLED` 后，读写都拒绝。

## 客户端

登录页在 OTP 和已设置的 2FA 都通过之后才看上面的码。永久停用进入处置页，不进聊天列表。只读和功能限制保持登录，被拒绝的操作展示状态和北京时间，不显示成网络错误。

白名单（只读冻结和永久停用的处置凭证都保留）：提交申诉、补充说明、身份验证、阅读 777000 通知、退出。

## 通知投递

处罚行和 outbox 行在同一次数据库事务里提交。事务失败则两边都没有。事务成功之后才调用 777000 的 `MsgPushUserMessage`。

因此：

- 库提交成功、投递失败：处罚有效，outbox 状态是 `retry`，进程重启后 `Flush` 再投。
- 对方已收下，但状态更新没写上：下次会再投一次。这是至少一次，不是外部 exactly-once。
- 幂等键是 `案件编号:事件`。已标记 `sent` 的键不会再投。

通知发送者固定是用户 777000，不看用户名是否叫 admin。

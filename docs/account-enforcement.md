# 账号处罚、解封与申诉

主线基线 `e2868f536cad6633931b210c8e88aa42ebe5f66c`。本文件描述 `pkg/enforcement`。它没有接入生产，没有对真实用户发通知，也没有把 7696 词库接进来。

## 2026-11-09 00:17 产品决定

- 处罚生效后，隔离环境里用合成账号验证封禁、临时限制和解封。
- 处罚与解封通知由已经存在的系统用户 **777000**（`@admin`）发送。不新建第二个 admin。
- 申诉由已经存在的 **777001**（`@tellomi`，Tellomi AI）接收并判定。AI 可以维持或解除**当前这一条**仍生效的处罚，不要求每一条申诉都先人工批准。
- 模型不可用、超时或证据不足时，申诉留在 `pending_review`。不加重处罚，不编造结论。
- 人工复核员仍可追加撤销，用来纠错。发起人不能批自己的永久处罚。
- 1/3/7/30 天档位、双审天数阈值、申诉 SLA 仍未批准。代码不把这些数字写成唯一档位。只有调用方给出的结束时间会生效。永久处罚必须由另一名 `senior` 批准后才生效。
- `TELLOMI_ENFORCEMENT_STORE` 未设置为 `memory` 时，进程不安装这套逻辑，登录和发消息保持原样。
- `TELLOMI_ENFORCEMENT_CREATE` 不是 `1` 时，不能新建处罚或标识限制。已经存在的记录继续执行。

## 开关

| 变量 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_ENFORCEMENT_STORE` | 空 | `memory` 不会安装。这个名字只存在于测试，不能当生产开关 |
| `TELLOMI_ENFORCEMENT_CREATE` | 空 | `1` 才允许新建。关掉不清空已有记录 |
| `TELLOMI_ENFORCEMENT_MYSQL` | 空 | 指向已经 `Migrate` 过的单独库。空着不安装。启动不建表 |
| `TELLOMI_ENFORCEMENT_SERVICE_KEY` | 空 | 凭证复查的服务间密钥，至少 32 字节的标准 base64。空着或无效时该端点拒绝，不放行 |
| `TELLOMI_ENFORCEMENT_E2E` | 空 | 只有 `1` 才在 `127.0.0.1:8819` 挂隔离演练路由。生产不要设置 |

持久记录只在 `TELLOMI_ENFORCEMENT_MYSQL` 指向单独的库，并且该库已经显式执行过 `Migrate` 时才打开。`OpenMySQL` 和进程启动不建表。聊天库名 `teamgram` 会被拒绝。表不进 `teamgramd/deploy/sql`。

正常回滚是换回上一份二进制，不删除处罚、审计或 outbox。数据保留。仍带这份逻辑且指向同一库的二进制会继续执行已有处罚。没有生产用的 `DROP TABLE`。测试只删除它自己创建的 `tellomi_enforcement_test` 库，不能对其他库执行。

内存 `New` 只给本包测试。进程安装路径不调用它。

通知与案件同事务写入 outbox。投递发生在提交之后。失败保持 `retry` 并可在重启后继续。已成功的键不重投。若消息系统已接收但状态更新丢失，会再投一次。这是至少一次，不是外部 exactly-once。契约见 [api/enforcement-contract.md](api/enforcement-contract.md)。

## 身份

| 用户 | 用户名 | 用途 |
| --- | --- | --- |
| 777000 | `@admin` | 处罚、解封、申诉受理和结果通知。发送走与登录通知相同的 `MsgPushUserMessage`，`UserId` 固定为 777000 |
| 777001 | `@tellomi` | 申诉判定。接口是 `pkg/tellomiai.Client.CompletePrompt`。本仓测试使用假 `Judge`，不是百炼联调 |

通知在内存 outbox 里按 `处置编号:事件` 幂等。发送失败保持 `retry`，处罚不回滚。`Flush` 再送。已经 `sent` 的键不会再送。

永久停用签发短时处置凭证（默认 15 分钟，技术有效期，不是处罚档位）。凭证不是普通 auth key，不能用来发消息。持有人可以读 777000 通知正文并提交申诉。普通会话在下一次受保护 RPC 上收到 `401 ACCOUNT_PERMANENT_DISABLED`。安装了 session revoker 时，批准永久处罚会调用现有 `AuthsessionResetAuthorization`。

## 登录与错误

发码响应不带处罚类型或期限。OTP 和已设置的 2FA 都通过之后才判定。

| 码 | 含义 |
| --- | --- |
| `IDENTIFIER_RESTRICTED` | 这一个手机号或邮箱本身不能登录。账号状态不变 |
| `ACCOUNT_READ_ONLY` | 可以登录，写操作拒绝 |
| `ACCOUNT_SCOPED_RESTRICTION` | 可以登录，只拒绝列出的范围 |
| `ACCOUNT_PERMANENT_DISABLED` | 不签发普通会话 |
| `ACCOUNT_SECURITY_LOCK` | 安全锁。没有处罚记录，没有处罚通知 |

`PHONE_NUMBER_BANNED` 仍只留给原来的空插件，不拿来表示账号处罚。

同一 `user_id` 的手机号和邮箱走 `LoginStage`。已绑定标识不能走 `auth.signUp` 新建账号。

## AI 判定

模型只收到：案件编号、规则版本、条款、原因类别、证据引用、用户补充。用户补充是不可信文本。输出只能是 `uphold`、`lift`、`insufficient`。别的动作（包括新建永久处罚）改记为待复核。`lift` 只结束申诉指向的那一条仍生效案件。并行处罚不动。证据引用里带 `reporter` 或 `private_chat` 的项会被丢掉。

## 覆盖

| 需求 | 这一批 |
| --- | --- |
| REQ-ID-01 / REQ-EML-01 手机号与邮箱同一账号 | `TestPhoneAndEmailShareTheCase` |
| REQ-ID-03 / REQ-ID-06 / REQ-ID-07 标识限制不连坐 | `TestIdentifierDoesNotPunishAccount` |
| REQ-ID-05 已绑定标识不能新注册 | `SignupInstead` |
| REQ-ST-02 功能限制仍可登录 | `TestReadOnlyWhitelistAndScoped` |
| REQ-ST-03 只读与申诉白名单 | 同上 |
| REQ-ST-04 / REQ-SES-05 永久停用与处置凭证 | `TestPermanentNeedsOtherSeniorAndGrant` |
| REQ-ST-05 安全锁不是处罚 | `TestSecurityLockIsNotASanction` |
| REQ-ST-06 并行与到期不误恢复 | `TestExpiryOverlapAndIdempotentNotice` |
| REQ-AUTH-01 2FA 前不展示处罚 | `TestPermanentNeedsOtherSeniorAndGrant` |
| REQ-AUTH-02 / REQ-AUTH-03 登录、注册、QR、发消息、资料、换绑、发现凭证、已有会话 | 各 handler 调用 `pkg/enforcement` |
| REQ-TERM-05 到期幂等 | `TestConcurrentExpireOneNotice` |
| REQ-NTF-03 通知失败与去重 | `TestNoticeRetryThenDedup`、`TestAdminPushUses777000` |
| REQ-ADM-02 双人审批 | `TestPermanentNeedsOtherSeniorAndGrant` |
| REQ-REL-01 / REQ-REL-03 关闭新建不清空 | `TestCreateOffWritesNothing`、`TestExistingRecordSurvivesCreateSwitch` |
| REQ-REL-02 读失败时写保护、申诉仍可用 | `TestFailClosedUsesLastSnapshot` |
| AI 维持 / 解除 / 证据不足 / 异常 / 注入 / 重复 | `TestAILiftUpholdInsufficientAndFailure`、`TestModelJudgeParsesAndRejectsGarbage` |

未在本仓声称通过：真机三端、百炼真实调用、管理后台页面、外部邮件、旧客户端文案。隔离作业 `TestAccountEnforcementBanAndLift` 用 `e2e/fakebailian/server.py`，那是本地假百炼，不是 DashScope。

## 跨仓

- content-engine：本仓在永久停用时不再签发 `discover.getEngineCredential`。已经发出的 JWT 要由引擎在读/写时向本仓复查，或本仓后续把处罚版本写进 JWT。这一批没有改引擎。
- Android / iOS：需要认识上面五个错误码，并在永久停用后走处置页。现有「此手机号码已被封禁。」不能当作账号处罚文案。这一批没有改客户端。
- `pkg/tellomiai`：把 `*tellomiai.Client` 赋给 `enforcement.ModelJudge.Completer` 才是真实模型。测试里的 `fixedJudge` / `scripted` 不是联调。客户端回归里的 `TELLOMI_AI_BASE_URL` 指向 `fakebailian`，模型名是 `e2e-fake-bailian`。

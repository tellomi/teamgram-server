# 账号处罚、解封与申诉

本文件描述 `pkg/enforcement`。7696 词库不参与判定。生产开关是 `TELLOMI_ENFORCEMENT_STORE=mysql`，库由操作员单独建好并执行一次迁移之后才会安装。未安装时登录和发消息保持原样。

## 2026-11-09 00:17 产品决定

- 处罚生效后，隔离环境里用合成账号验证封禁、临时限制和解封。
- 处罚与解封通知由已经存在的系统用户 **777000**（`@admin`）发送。不新建第二个 admin。
- 申诉由已经存在的 **777001**（`@tellomi`，Tellomi AI）接收并判定。AI 可以维持或解除**当前这一条**仍生效的处罚，不要求每一条申诉都先人工批准。
- 模型不可用、超时或证据不足时，申诉留在 `pending_review`。不加重处罚，不编造结论。
- 人工复核员仍可追加撤销，用来纠错。发起人不能批自己的永久处罚。
- 1/3/7/30 天档位、双审天数阈值、申诉 SLA 仍未批准。代码不把这些数字写成唯一档位。只有调用方给出的结束时间会生效。永久处罚必须由另一名 `senior` 批准后才生效。
- `TELLOMI_ENFORCEMENT_STORE` 不是 `mysql` 时，进程不安装这套逻辑，登录和发消息保持原样。`memory` 只出现在测试里，不会安装。
- `TELLOMI_ENFORCEMENT_CREATE` 不是 `1` 时，不能新建处罚或标识限制。已经存在的记录继续执行。
- `TELLOMI_ENFORCEMENT_REVIEW` 不是 `1` 时，举报不会送给管理员确认，内容接口也不做判定。

## 开关

| 变量 | 默认 | 作用 |
| --- | --- | --- |
| `TELLOMI_ENFORCEMENT_STORE` | 空 | `mysql` 才安装持久库。`memory` 和空值都不安装 |
| `TELLOMI_ENFORCEMENT_CREATE` | 空 | `1` 才允许新建。关掉不清空已有记录 |
| `TELLOMI_ENFORCEMENT_MYSQL` | 空 | 单独库的 DSN，不能是 `teamgram`。库要先执行过迁移。空着不安装。启动不建表 |
| `TELLOMI_ENFORCEMENT_REVIEW` | 空 | `1` 才把属实的账号举报交给管理员，并打开内容判定 |
| `TELLOMI_ENFORCEMENT_ADMINS` | 空 | 逗号分隔的用户名，一开始是 `erel`，可以继续追加。只有这些人回复的「是」或「否」生效 |
| `TELLOMI_ENFORCEMENT_CONFIDENCE` | `0.8` | 内容自主判定的置信度下限。低于它就交给管理员 |
| `TELLOMI_ENFORCEMENT_SERVICE_KEY` | 空 | 凭证复查和内容接口的服务间密钥，至少 32 字节的标准 base64。空着或无效时该端点拒绝，不放行 |
| `TELLOMI_ENFORCEMENT_ADDR` | 空 | `0.0.0.0:8820` 才在容器网络里监听。`off` 或空着不监听。不要把这个端口映射到宿主机 |
| `TELLOMI_ENFORCEMENT_E2E` | 空 | 只有 `1` 才在 `127.0.0.1:8819` 挂隔离演练路由。生产不要设置 |

真实百炼还要 `TELLOMI_AI=1`、`TELLOMI_AI_API_KEY`、`TELLOMI_AI_BASE_URL`（默认 `https://dashscope.aliyuncs.com/compatible-mode/v1`）、`TELLOMI_AI_MODEL_PRIMARY`（默认 `qwen-plus`）。备用模型是 `TELLOMI_AI_MODEL_SMALL` 和 `TELLOMI_AI_MODEL_FALLBACK`。测试指向 `e2e/fakebailian`，不调用百炼。

## 管理员确认

用户走现有举报（`TELLOMI_REPORT_*`，发现举报仍转到 `teamgram:8816`）。`TELLOMI_ENFORCEMENT_REVIEW=1` 时，Tellomi AI 对照被举报消息、资料和以往举报做判断。属实才由用户 777001（`@tellomi`）私聊给 `TELLOMI_ENFORCEMENT_ADMINS` 里的账号，正文有证据摘要、被举报用户、理由和案件号，请对方回复「是」或「否」。

在和 `@tellomi` 的私聊里，允许名单上的账号只有回复「是」才封禁，只有回复「否」才关闭且不封禁。比较时只去掉首尾空白，正文必须正好是这两个字之一。「是的」、yes、y、no、n、带案件号的句子以及其他文字都不会封禁，也不会关闭案件：案件保持待确认，并收到一句短提示。有多起待确认时，要回复对应的那一条，不能靠在正文里写案件号来选定。名单外的人说「是」不会封禁。

封禁走现有逻辑：AI 以 `agent` 发起永久处罚，管理员以 `senior` 批准。777000 发通知，登录和发消息按永久停用拒绝，申诉仍由 777001 判定。每次 AI 结论和管理员答复都写入 `audit_log`（谁、何时、理由、答复）。

内容（帖子和评论）由 AI 自主决定。置信度达到 `TELLOMI_ENFORCEMENT_CONFIDENCE` 的 `remove` / `keep` 直接返回给 content-engine。拿不准的走同一套管理员确认。content-engine 的改动不在本仓，接口见 [api/enforcement-contract.md](api/enforcement-contract.md)。

## 一次迁移

启动不建表。操作员在宿主机先建库和用户，部署含 `/app/bin/enforcement-migrate` 的镜像之后执行一次：

```bash
docker compose -p tellomi \
  -f docker-compose-env.yaml -f docker-compose.yaml \
  -f docker-compose.geoip.yaml -f docker-compose.tellomi.yaml \
  exec -T teamgram /app/bin/enforcement-migrate
```

然后再创建一次 teamgram 容器，进程才能在已有表上安装。再跑一次是空操作，不删数据。库名 `teamgram` 会被拒绝。没有生产用的 `DROP TABLE`。

持久记录只在 `TELLOMI_ENFORCEMENT_STORE=mysql` 且 `TELLOMI_ENFORCEMENT_MYSQL` 指向单独的库，并且该库已经显式执行过 `/app/bin/enforcement-migrate` 时才打开。`OpenMySQL` 和进程启动不建表。聊天库名 `teamgram` 会被拒绝。表不进 `teamgramd/deploy/sql`。

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

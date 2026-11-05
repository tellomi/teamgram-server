# AGENTS.md — teamgram-server

本仓职责：Tellomi 的 MTProto 服务端（`tellomi/teamgram-server`），实现官方客户端实际调用的 Telegram API。

## 必读（按顺序）

1. `docs/TELLOMI_RULES.md` —— 铁律（四仓同文，改规则开 PR）
2. 模块边界：https://github.com/tellomi/tellomi/blob/main/docs/MODULE_BOUNDARIES.md
3. `docs/api-coverage.md` —— 官方四端调用与服务端状态；生成器 `go run ./tools/apicoverage`
4. `docs/architecture-extension.md` —— 独立服务的接入缝，数据不进聊天库

## 禁区

- 不提交密钥、token、生产 `.env`。
- 生产 Redis 禁 `FLUSHALL`。
- 补服务器 API 对照官方 TL/API 文档。「已实现」= 路由 + handler。不拍脑袋 stub。
- API 覆盖只跟官方四端实际调用。改 RPC 必须重跑 `go run ./tools/apicoverage`，并把 `docs/api-coverage.md` 与 `docs/api-coverage.csv` 放进同一 PR。
- 不往聊天库（`users`、`messages`、dialog）加独立产品的表。swipe / moments 用自己的库，见 `docs/architecture-extension.md`。

## 合入 / 出包闸门

- CI 绿 + 评审后 squash 合入 `master`；不直推默认分支。
- `master` 保护要求这四项检查（名称见 `docs/testing.md`）：`API coverage / coverage`、`Client regression / e2e`、`Compose config / minio-profile`、`Go test / go-test`。
- 改或新增 RPC：`e2e/client` 用例与覆盖文档进同一 PR。
- 跨仓契约变更：契约 PR 先合，实现 PR 同批合齐。

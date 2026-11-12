# AGENTS.md — teamgram-server

本仓职责：Tellomi 的 MTProto 服务端（`tellomi/teamgram-server`），实现官方客户端实际调用的 Telegram API。

## 必读（按顺序）

1. 本文件「永久开发规范」（Telegram Native-First）—— **仓库内权威版本**；改规范开 PR
2. `docs/TELLOMI_RULES.md` —— 铁律（四仓同文，改规则开 PR；已同步收录本规范摘要）
3. 模块边界：https://github.com/tellomi/tellomi/blob/main/docs/MODULE_BOUNDARIES.md
4. `docs/api-coverage.md` —— 官方四端调用与服务端状态；生成器 `go run ./tools/apicoverage`
5. `docs/architecture-extension.md` —— 独立服务的接入缝，数据不进聊天库

## 永久开发规范（Telegram Native-First）

本规范对所有为 Tellomi 写代码的人与 AI **永久生效**。仓库内的权威版本是**本仓根目录 `AGENTS.md` 的「永久开发规范」章节**。`docs/TELLOMI_RULES.md`（如有）同步收录原则与检查清单。改本规范必须开 PR，不要只改聊天记忆。

Telegram 客户端已经发展十多年，交互、缓存、媒体和体积控制都经过长期打磨。Tellomi 是在这套代码上做品牌与功能扩展，不是另写一套 App。增加功能或修复问题，必须先服从下面的顺序。

## 原则

### 1. 先读原版，再动手

增加任何功能之前，必须先查看、阅读并理解 Telegram 原有代码的标准、结构和设计规范。相关平台要对照着看（移动端至少看 iOS 和 Android，不能只看一端）。没有完成阅读，不得开始写新代码。

### 2. 原版已有的能力，必须优先复用

先确认原代码有没有同一类功能。有，就先使用原有功能代码，不要再实现一套平行逻辑。

典型例子：我们在「动态」（Moments）里查看图片。Telegram 已经有完整的图片/视频查看器、手势、保存和共享媒体列表。必须先走原有查看器，而不是为动态新写一套看图页。

资料页标签也是同样的规则：Telegram 原生是「有数据才出现对应标签」。动态、快拍等扩展必须先读这段原生实现，再按同样规则接入，不要自创「标签永远显示」，也不要把快拍与动态合并。资料页最多同时出现一个「快拍」和一个「动态」；中文里 Stories 必须叫「快拍」。

### 3. 只有原版不够用时才扩展，扩展仍须遵循原版规范

只有在确认原实现覆盖不了产品需求时，才允许新增扩展代码。扩展必须遵循 Telegram 的代码结构、命名、线程模型、导航和生命周期习惯，并尽量从原有类型上长出来，而不是另起一套架构。

### 4. 瘦安装、肥运行；接入原有缓存与下发机制

Telegram 的设计是：安装包保持精简，贴纸、主题、文案、配置和媒体等大多由服务端下发，本地文件和缓存会自动清理。新功能必须沿用这一理念：

- 不要把大资源（视频、大量图片、字体、主题包、未使用的原生库或二进制）打进安装包。
- 配置、文案、资源能走服务端下发或原有 langpack / 远程配置的，不要写死在客户端包里。
- 必须接入 Telegram 原有的缓存与存储管理体系，让新文件同样被统计、过期和清理。禁止自建不受该体系管理的本地目录或「影子缓存」。

### 5. 能不修改 Telegram 原有源码，就不修改

大多数线上问题应先从服务端查起（协议实现、API 覆盖、数据、权限、媒体、限流）。Telegram 客户端代码已经非常成熟，不要先入为主地改客户端。

不得不改原版文件时：

- 只做最小改动（最小 diff），优先在扩展目录或既有挂钩里完成；
- 按本仓既有做法登记到对应的 `UPSTREAM_PATCHES` 文件（见下方本仓路径）；
- 在 PR 里写清改了哪些原版文件、为什么不能只改扩展或服务端。

### 6. 修复必须有真实测试；禁止桩代码和假成功响应

每个修复、每个行为变更都要有真实测试（单元测试、协议测试或可复现的集成验证）。禁止用空实现、假 200、伪造成功返回来「先过关」。测试失败时，不能靠放宽断言混过去。

## 开工前检查清单（必须书面回答，并写入 PR 描述）

在写第一行功能代码之前，以及在打开 PR 时，必须明确回答下列问题：

1. **原版有没有类似功能？** 有或没有。若有，写明模块、文件路径和准备复用的类型或函数。
2. **若有，为什么原版不够用？** 若够用，写明如何复用，以及本次是否没有新写平行实现。
3. **服务端能不能解决？** 若能，为什么还要改客户端；若不能，服务端缺的是哪一段协议或数据。
4. **本次改了哪些 Telegram 原版文件？** 列出路径。若未改原版，写「未改原版」。
5. **新文件或缓存是否接入了原有存储与自动清理？** 有没有把大资源打进安装包？
6. **测试是什么？** 指出测试文件或验证步骤。确认不是桩代码，也不是假 200。

没有这六条答案的 PR，不得进入复审，更不得合入。


## 本仓（teamgram-server）对应要求

本仓是 MTProto 服务端，原则同样生效，侧重点如下：

- 优先兼容 Telegram 原生协议和官方客户端的实际行为，而不是为了迁就自创的客户端逻辑去改协议语义。
- **客户端出现异常时，先查服务端**（API 是否按官方 TL 实现、路由和 handler 是否完整、数据、权限、媒体、限流），再考虑改客户端。Telegram 客户端已经非常成熟，大多数问题出在服务端实现或数据。
- 补 API 必须对照官方 TL / API 文档。「已实现」= 路由 + handler，禁止桩代码和假成功响应。改 RPC 必须重跑 `go run ./tools/apicoverage`，并把 `docs/api-coverage.md` 与 `docs/api-coverage.csv` 放进同一 PR。
- 不要往聊天库（`users`、`messages`、dialog）加独立产品的表；swipe / moments 用自己的库，见 `docs/architecture-extension.md`。
- 原版/上游改动登记：`UPSTREAM_PATCHES.md`。
- 合入必须独立复审 PASS + CI 绿；作者不自合。生产 Redis 禁止 `FLUSHALL`。


## 禁区

- 不提交密钥、token、生产 `.env`。
- 生产 Redis 禁 `FLUSHALL`。
- 补服务器 API 对照官方 TL/API 文档。「已实现」= 路由 + handler。不拍脑袋 stub。
- API 覆盖只跟官方四端实际调用。改 RPC 必须重跑 `go run ./tools/apicoverage`，并把 `docs/api-coverage.md` 与 `docs/api-coverage.csv` 放进同一 PR。
- 不往聊天库（`users`、`messages`、dialog）加独立产品的表。swipe / moments 用自己的库，见 `docs/architecture-extension.md`。

## 合入 / 出包闸门

- CI 绿 + 评审后 squash 合入 `master`；不直推默认分支。
- `master` 保护要求这些检查（名称见 `docs/testing.md`）：`API coverage / coverage`、`Client regression / e2e`、`Compose config / minio-profile`、`Go test / go-test`、`Server reliability / reliability`。
- 改或新增 RPC：`e2e/client` 用例与覆盖文档进同一 PR。
- 跨仓契约变更：契约 PR 先合，实现 PR 同批合齐。

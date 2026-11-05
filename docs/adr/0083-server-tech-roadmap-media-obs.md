# ADR-0083 服务端技术路线：媒体身份与观测

| | |
|---|---|
| 状态 | 已接受。Owner 批准的服务端路线，2026-11-05 |
| 日期 | 2026-11-05 |
| 仓 | `tellomi/teamgram-server`（MTProto 服务端） |
| 替代 | 不替代 ADR-0082。0082 是 P1 决策；本文是它之后的路线 |
| 部署 | 本文不部署、不切流量、不改生产 `.env` |

总仓 ADR-0082（`tellomi/tellomi` 的 `docs/adr/0082-media-dedup-telegram-baseline.md`）不在本仓，本 PR 改不到那份正文，也不改写它已经写下的决定。本仓入口是 [media-dedup.md](../media-dedup.md)。若以后在总仓 0082 末尾加指针，只追加下面这一句，不要改 0082 已有段落：

> 后续 / Follow-ups：服务端波次、观测与明确不做见 teamgram-server `docs/adr/0083-server-tech-roadmap-media-obs.md`。

## 背景

Tellomi 的官方客户端走 MTProto。服务端是 Teamgram 这一套：gnetway → session → bff → dfs / media，对象在 MinIO 或 Cloudflare R2（见 [storage.md](../storage.md)）。聊天库仍是 MySQL。扇出仍是 Kafka（`Sync-T`、`Inbox-T`）。

生产在成都。已装客户端从 `help.getConfig` 的 `dc_options` 拿数据中心，值是 IP 和端口，不是域名。当前生产列表是 `1:8.137.39.82:10443` 与 `1:8.137.39.82:5222`（[DC_ADDRESSES.md](../ops/DC_ADDRESSES.md)）。域名留给官网和协议页：预发 `staging-api.tellomi.cn`，生产 `api.tellomi.cn`；MTProto `10443`，WebSocket `11443`（`docs/TELLOMI_RULES.md`）。

两条约束写在路线外面，后面的波次都不能碰：

1. **没有 Owner 的书面同意，不切 DNS。** 不把 DC 列表改成域名，不撤掉 `8.137.39.82`，不用本路线当理由去改解析。
2. **D-23 继续搁置。** 本仓没有 D-23 正文。本文不解释它、不执行它。要做 D-23 必须另写，不能从本文推断。

P1 已经合入：上传合并后按 `sha256 + size` 复用对象，每条 document / photo 仍有自己的 `id`、`access_hash`、`file_reference`。命中 hash 不是授权。见基线一节。

## 决策

| 项 | 决定 |
|---|---|
| 内容身份 | 内容寻址。同一 `storage_class` 内，唯一键是 `sha256 + file_size`。不做感知哈希（pHash 一类）。 |
| 授权 | `access_hash` 按 document / photo **行**发，不按 blob 发。上传命中只复用对象键，新建一行。 |
| 覆盖面 | 把这条身份扩到照片、GIF、普通文件。视频字节走 document 路径，与普通文件同一张 `media_blobs`。 |
| 客户端跳过上传 | 官方 `messages.getDocumentByHash`：`sha256`（32 字节）+ `size` + `mime_type`。命中返回已有 Document（它自己的 `access_hash`），否则 `documentEmpty`。当前构造 `#b1f2061f`（size 为 long）。旧构造 `#338e2464`（size 为 int）不要新实现。 |
| P2 | 实现 `upload.getFileHashes`，并完成引用计数 GC：还有引用就不删对象；最后一条引用按保留策略放掉之后再删对象。 |
| 静图 | 静图衍生尺寸用 govips。GIF 与视频不进 govips。 |
| 观测 | 上传与去重打 OpenTelemetry。沿用 `TELLOMI_OTEL_ENDPOINT`；变量为空则不装导出器。 |
| 通话与推送 | LiveKit、ntfy 推迟。通话维持 `phone.*` + 成都 coturn（[calls.md](../calls.md)）。推送维持 APNs / FCM / 华为 / 小米（[push.md](../push.md)）。 |
| 现在不引入 | IPFS、Matrix、SeaweedFS、全量 E2EE、Kafka 换成 NATS、MySQL 换成 TiDB。见波次 E。 |

照片不走 `messages.getDocumentByHash`。`photos.sha256` / `photos.blob_id` 只服务同一套对象引用。`md5_checksum` 只做完整性校验，不是复用键。加密上传（`dfs.uploadEncryptedFileV2`）比对 md5，不符则 `MD5_CHECKSUM_INVALID`；`encrypted_files.md5_checksum` 不是去重键，加密文件不进跨用户 `media_blobs`。

## 基线（已合入，不要重做）

| | |
|---|---|
| PR | [#212](https://github.com/tellomi/teamgram-server/pull/212) |
| 合入提交 | `d7bc091`（`d7bc0914ed12009d21becd670e6f7ad71cef98b3`），2026-11-05 |
| 迁移 | `teamgramd/deploy/sql/migrate-20261110-media-dedup.sql` |
| 代码 | `app/service/mediablob`；`app/service/dfs/internal/core/dedup.go` |
| e2e | `TestMediaDedupTwoAccounts` |

表（真实名字）：

| 表 | 作用 |
|---|---|
| `media_blobs` | 唯一键 `(storage_class, sha256, file_size)` → 对象键、`ref_count`、第一份上传的 id |
| `media_blob_refs` | 每条 document / photo 指向一个 blob |
| `media_photo_paths` | 复用图片的各档尺寸指向已有对象键 |
| `documents.sha256` / `blob_id` | `messages.getDocumentByHash` 的查找列 |
| `photos.sha256` / `blob_id` | 图片引用，不参与 getDocumentByHash |

dfs 与 media 都要连到这块 MySQL（`dfs.yaml` 的 `Mysql`，与 media 同一个库）。DSN 为空时保持旧行为：每次上传写 `<id>.dat`。

已接到 Claim 的上传：

| 路径 | 存储类 |
|---|---|
| `dfs.uploadPhotoFileV2` | `photos`，原图键 `0/<第一份 id>.dat` |
| `dfs.uploadGifDocumentMedia` | `documents`，键 `<第一份 id>.dat` |
| `dfs.uploadDocumentFileV2` | `documents`（普通文件） |
| `dfs.uploadMp4DocumentMedia` | `documents`（含 `video/mp4`） |

`upload.getFileHashes` 已实现（ADR-0083 B2）：按 location 读明文，128 KiB SHA-256，`offset` 与 `upload.getCdnFileHashes` 同一口径。CDN 那条只哈希密文，不参与上传去重。

`Store.Release` 删掉引用行。还有别的引用就只减 `ref_count`。最后一条引用在同一事务里删掉 `media_blobs` 行（不留下 `ref_count = 0`）并把对象键写入 `media_blob_gc`。对象删除在事务提交之后；失败留下孤儿对象和 gc 行，下次保留任务再删。删一条消息不会放引用，也不会删对象。保留任务与账号清理在仍有别的 `media_blob_refs` 时不删对象（[media-retention.md](../media-retention.md)）。静图衍生仍在 `app/service/dfs/internal/imaging`，仓库里没有 govips。

## 波次与验收

建议窗口见下一节。验收没过就停在该波，不把下一波的库或依赖提前加进来。改了 RPC 的波必须带 `e2e/client` 用例，并重跑 `go run ./tools/apicoverage`，把 `docs/api-coverage.md` 与 `docs/api-coverage.csv` 放进同一 PR。

[media-dedup.md](../media-dedup.md) 里的 P3（`file_reference` 刷新、分块并行与断点续传、CDN DC、按需缩略图）不在 A–E 的承诺里。要做就另列，并写入 `docs/api-coverage.md`。

### A — 服务端照片 / GIF / 文件

把基线已经接上的三条路径收成可验收的同一套身份。不新造表，不换对象存储。

验收：

1. 同一张静图，两个账号各发一次：`photos` 两行，`access_hash` 不同；`media_blobs`（`storage_class = photos`）一行；两条都能 `upload.getFile`。
2. 同一 GIF，两个账号各发一次：`documents` 两行，`access_hash` 不同；`media_blobs`（`documents`）一行。GIF 含 `image/gif` 与现有动画 mp4 路径。
3. 同一普通文件（非图片 mime）同样复用，且 `messages.getDocumentByHash` 用该 `sha256 + size + mime` 命中其中一条 Document。mime 不符返回 `documentEmpty`。
4. 上传命中不为本次上传者改写已有行的 `access_hash`。照片不从 `messages.getDocumentByHash` 返回。
5. `Mysql` DSN 为空时仍写 `<id>.dat`。
6. e2e 覆盖照片、GIF、普通文件。现有 `TestMediaDedupTwoAccounts`（视频）保持通过。

### B1 — 客户端按 hash 跳过上传

服务端契约就是基线里的 `messages.getDocumentByHash`。本波的行为在客户端：发 GIF、贴纸、已知文件之前先算 sha256，再调用该方法。

验收：

1. 命中：用返回的 Document（它自己的 `access_hash` 与 `file_reference`）发消息，不再为同一批字节调用 `upload.saveFilePart`。
2. 未命中：走现有上传。服务端仍按 A 的规则建新行。
3. 调用者必须自带 `sha256`、`size`、`mime`。没有「按文件名找」或「按看起来像的图找」。
4. 私聊照片不改成跨用户领取别人的 photo `access_hash`。
5. Android 与 iOS 各有一条可重复的记录（客户端仓的用例或书面步骤）。本仓只有在契约变化时才改 RPC。

客户端改动在 `tellomi/Telegram` 与 `tellomi/Telegram-iOS`，不在本 ADR 的 PR 里。

### B2 — P2：`getFileHashes` 与引用计数 GC

验收：

1. `upload.getFileHashes` 有路由和 handler，不再返回 `ENTERPRISE_IS_BLOCKED`。分块 SHA-256 与 `upload.getCdnFileHashes` 同一口径。`offset` 有效。
2. 同一对象仍有别的 `media_blob_refs` 时，保留任务与任何删除路径都不删对象。另一条消息的 `upload.getFile` 仍成功。
3. 最后一条引用被保留策略放掉之后，对象被删除。`media_blobs` 不留下 `ref_count = 0` 的活行。删除失败时宁可留下孤儿对象，也不留下指向已删对象的活引用。
4. 删一条消息仍然不立刻删对象。保留天数、订阅豁免、缩略图保留仍按 [media-retention.md](../media-retention.md)。
5. e2e 与覆盖文档在同一 PR。`go run ./tools/apicoverage -check` 通过。

### C — govips、卫生与生命周期

静图（JPEG / PNG / WebP 的衍生尺寸）改用 govips。GIF、视频、普通文件的字节路径不动，ffmpeg 仍管视频缩略图。

验收：

1. 客户端已经认识的尺寸类型（照片 `s` / `m` / `x` / `y` / `w`，以及保留策略留下的 `i` / `j`；文档缩略图）仍然能下。不新增客户端不认识的 size type。
2. hash 命中时不把衍生尺寸再写一份新对象。沿用 `media_photo_paths` 指向第一份的键，除非未决问题 1 另有书面结论。
3. govips 失败时不写入半截尺寸，不把坏图标成成功。失败行为与现在的错误一致（上传失败或不要预览），并有测试。
4. 上传中途失败不留下被新行引用的残缺对象。没有引用的孤儿对象可以留到 GC，不能被新的 `access_hash` 指到。
5. 保留任务在 `TELLOMI_MEDIA_RETENTION_DRY_RUN=true` 下先跑。`ref_count > 0` 的 blob 不出现在真正删除的集合里。订阅用户、头像、贴纸、GIF、壁纸的豁免不变。
6. 成都不编译带 govips 的镜像。镜像仍由 GitHub Actions 推到 ACR，成都只 pull（[ci-acr.md](../ci-acr.md)）。

### D — 上传与去重的 OpenTelemetry

沿用 [link-tracking-zh.md](../link-tracking-zh.md)。`TELLOMI_OTEL_ENDPOINT` 为空时不安装导出器，没有采集器也不打 `[otel] error`。只有采集器在跑时才设 `TELLOMI_OTEL_ENDPOINT=jaeger:4317`（OTLP gRPC）。

验收：

1. 端点有值时，一次上传的 trace 能看出 claim、命中或未命中、对象写入是否发生。属性至少有 `storage_class`、`reused`、字节大小、耗时。
2. `messages.getDocumentByHash` 的命中与 `documentEmpty` 能在 trace 里分开。
3. 属性里不放 `access_hash`、不放对象字节、不放用户明文。内容 `sha256` 不进导出属性（追踪后端不能变成按文件内容检索）。
4. 端点为空时，上传路径不因追踪失败而失败，日志里没有逐条 `[otel] error`。

### E — 推迟

没有窗口。下面每一项都要新的 ADR 加上 Owner 书面同意才能重开。本路线的波次 A–D 不得预埋这些依赖。

| 项 | 现在为什么不做 |
|---|---|
| LiveKit | 1:1 通话已是 `phone.*` + coturn。群通话用进程内 SFU。见 [calls.md](../calls.md)。 |
| ntfy | 推送已是 APNs / FCM / 华为 / 小米。见 [push.md](../push.md)。 |
| IPFS | 内容身份已经是库里的 `sha256 + size` 加现有对象存储。不另做 pin 与网关。 |
| Matrix | 客户端是 MTProto。不并行第二套聊天协议。 |
| SeaweedFS | 对象在 MinIO / R2，客户端是 `minio-go`。不迁移存储。 |
| 全量 E2EE | 秘密聊天仍由客户端加密。服务端对普通媒体做 sha256 复用与这条路线绑在一起；全量端到端会拆掉官方客户端的媒体与本 ADR 的身份。 |
| Kafka → NATS | 扇出仍是现有 Kafka topic。换总线是另一次迁移。 |
| TiDB | `media_blobs` 是 MySQL 上的小表。不换聊天库引擎。 |
| D-23 | 继续搁置。 |
| DNS 切换 | 没有 Owner 书面同意就不做。 |

## 建议日程

时区 Asia/Shanghai。日期是建议窗口，不是承诺。Owner 改窗口不用改上面的决策表。A 完成之前不开始 B。B1 与 B2 可以并行。C 使用 B2 的引用计数语义。D 等 A 的上传路径稳定后再收口，可以与 C 的后半段重叠。

| 波次 | 建议窗口 | 依赖 | 出口 |
|---|---|---|---|
| A 服务端照片 / GIF / 文件 | 2026-11-05 ～ 2026-11-18 | #212 已合 | 三种媒体的 e2e |
| B1 客户端 hash 跳过上传 | 2026-11-19 ～ 2026-12-03 | A 的 getDocumentByHash 不再改形 | 第二次发送不再传整文件 |
| B2 P2：getFileHashes + 引用计数 GC | 2026-11-19 ～ 2026-12-10 | A | 方法可用；有引用不删对象 |
| C govips + 卫生 + 生命周期 | 2026-12-08 ～ 2026-12-24 | A 的图片路径；B2 的计数 | 静图尺寸仍可下；dry-run 不误删 |
| D 上传 / 去重 OTel | 2026-12-15 ～ 2027-01-06 | A 的上传路径 | 命中与未命中可在 trace 里分开 |
| E 推迟项 | 无 | Owner 另文 | 本 ADR 明确不做 |

本仓没有 `docs/schedule/`。日程只放在本文，不另抄一份。

## 安全不变量

实现 A–D 时这些条保持为真：

1. 命中 hash 不是授权。新的 document / photo 行自带 `id`、`access_hash`、`file_reference`。
2. 上传路径不把别人的 `access_hash` 交给本次上传者。`messages.getDocumentByHash` 只把调用者已经持有的 `sha256 + size + mime` 换成那条 Document 自己的 `access_hash`。
3. 身份是 `sha256 + size`（再加 `storage_class`）。不引入感知哈希。`md5_checksum` 只做完整性。
4. 加密文件与秘密聊天文件不进跨用户 `media_blobs`。
5. 不知道 hash 就不能领取文档。不提供按文件名、按用户、按「像某张图」的查找。
6. 还有引用就不删对象。引用归零后的对象删除失败时留下孤儿，不留下悬空引用。
7. 追踪与日志不写 `access_hash`、不写对象字节、不写完整内容 `sha256`。
8. 生产 Redis 禁止 `FLUSHALL`。密钥、token、生产 `.env` 不进 git。
9. 没有 Owner 书面同意，不改 DC 的 IP:端口，不切 DNS，不执行 D-23。
10. 成都主机不编译镜像。

## 测试与发布清单

本文这一 PR 不改 RPC、不跑迁移、不部署。

每一波自己的 PR：

- [ ] 对照官方 TL。已实现 = 路由 + handler。不写空 stub 充数。
- [ ] 改了 RPC：`e2e/client` 用例与 `go run ./tools/apicoverage` 的两份覆盖文档在同一 PR。P0 / P1 方法进入 smoke 列表。
- [ ] `go test ./...` 通过。客户端能看见的行为有对应的 `Test...`，不只靠 smoke。
- [ ] 新 SQL 用新的 `teamgramd/deploy/sql/migrate-*.sql`，可重复执行。不改已经合入的 `migrate-20261110-media-dedup.sql` 的语义。
- [ ] `media_blobs` / `media_blob_refs` / `media_photo_paths` 留在现有媒体库。不往 `users`、`messages`、dialog 加独立产品的表。
- [ ] 保留任务先 `TELLOMI_MEDIA_RETENTION_DRY_RUN=true`。Owner 书面同意之前不在成都把 dry-run 关掉。
- [ ] 四项检查保持绿：`API coverage / coverage`、`Client regression / e2e`、`Compose config / minio-profile`、`Go test / go-test`（名称见 [testing.md](../testing.md)）。
- [ ] 合入 `master` 之后由 Actions 构建镜像并推 ACR。成都 pull，不在成都编译。
- [ ] 不改 `TELLOMI_DC_OPTIONS`，不切 DNS，不执行 D-23，不 `FLUSHALL`。

## 未决问题

这三件在对应波次里先按「现行做法」做，不在本文里假装已经选完。要改现行做法，在该波的 PR 里写明，并改本文的状态或另开 ADR。

1. **多档 PhotoSize 要不要各自成为 blob。** 现行：只对原图做 `sha256 + size`；衍生尺寸记在 `media_photo_paths`，指向第一份上传的对象键。另一选择是每一档尺寸自己一行 `media_blobs`。A 与 C 在没有新结论之前按现行做法。
2. **大视频要不要在客户端先算完整 sha256。** 现行：服务端在合并后的可 seek 上传上整段哈希（`claimDocumentReader`），再决定写不写。B1 的跳过上传适用于 GIF、贴纸和客户端已经算得起哈希的文件。多 GB 视频是否在上传前整段预哈希，本文不规定。没结论之前，大视频仍走上传，由服务端在合并后哈希。
3. **引用归零后的对象删除是同步还是异步。** B2 定为异步。`Release` 在同一事务里删引用行，计数归零时删 `media_blobs` 行，并把对象键写入 `media_blob_gc`。对象删除在提交之后，由保留任务（以及插入时返回的键）执行。崩溃窗口是：事务已提交、对象还没删。这时没有悬空引用，也没有 `ref_count = 0` 的活行，只留下孤儿对象和 gc 行；下次 live 跑会删对象。删除失败同样留下 gc 行，不把引用写回去。

## 相关文档

按这个顺序读：

1. 本文
2. [media-dedup.md](../media-dedup.md) — P1 行为与表
3. [media-retention.md](../media-retention.md) — 90 天、订阅豁免、dry-run
4. [storage.md](../storage.md) — 对象键与 R2 / MinIO
5. [link-tracking-zh.md](../link-tracking-zh.md) — `TELLOMI_OTEL_ENDPOINT`
6. [TELLOMI_RULES.md](../TELLOMI_RULES.md) — DC 用 IP:端口、生产 Redis、覆盖文档
7. [testing.md](../testing.md) — 四项合入检查
8. 总仓 ADR-0082 — P1 决策原文（不在本仓）

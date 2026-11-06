# ADR-0082 P3 现状

盘点提交：`66cc5b5e9c710133bbe9fa0ac3d92f5f06814bcb`（`master`，#219）。本文只记录这一提交上的代码现状，不实现这四项。

总仓 [ADR-0082](https://github.com/tellomi/tellomi/blob/main/docs/adr/0082-media-dedup-telegram-baseline.md) 阶段 P3 要求把下面四项 Telegram 公开技术逐项写进本仓文档，不在那份 ADR 里展开实现。P1（#212，`media_blobs` + `messages.getDocumentByHash`）和 P2（#215，`upload.getFileHashes` + 引用计数 GC）已经合入，不在这次盘点里重写。入口仍是 [media-dedup.md](media-dedup.md)。

分块上传对照总仓 [ADR-0031](https://github.com/tellomi/tellomi/blob/main/docs/adr/0031-resumable-media-upload.md)（`docs/adr/0031-resumable-media-upload.md`，2026-09-10）。本文不沿用 ADR-0082 里日期为 2026-11-05 的「现状」段。四项结论来自本提交的代码、`proto/schema/telegram_api_layer232.tl`，以及官方文档。和 ADR-0031 的对照写在第 2 节。

## 为什么不改生成的覆盖矩阵

`docs/api-coverage.md` 和 `docs/api-coverage.csv` 由 `go run ./tools/apicoverage` 生成。它只列出四端摘录 `tools/apicoverage/data/clients.json` 里实际调用的方法。生成器自己写明：改生成器，不要改表。CI 的 `go run ./tools/apicoverage -check` 会在这两份文件和代码树不一致时失败。

这四项对不上那张表：

- file_reference 刷新和按需缩略图不是一条 RPC，矩阵没有格子可填。
- `upload.saveFilePart`、`upload.saveBigFilePart`、`upload.getCdnFile`、`upload.getCdnFileHashes`、`upload.reuploadCdnFile`、`help.getCdnConfig` 已经在矩阵里，状态是 `implemented`。那个词的意思是：会话路由到了 bff，并且 handler 会读写真状态或返回填好的结果。它不表示 P3 这一项已经做完。

所以这次不改 `docs/api-coverage.md`、`docs/api-coverage.csv`、`tools/apicoverage/data/clients.json`，也不改生成器。缺口写在本文。

状态用这四个词：`implemented` 路由和 handler 都在，并且行为对上官方这一项；`partial` 有真 handler 或真存储，但官方这一项只做了一截；`stub` 有路由，成功返回空对象；`missing` 没有这一项的行为。

## 1. file_reference 刷新 — missing

官方没有单独的刷新 RPC。`file_reference` 是 `document`、`photo` 以及对应 `Input*` 上的一段不透明字节。过期或无效时，服务端返回 `400 FILE_REFERENCE_EXPIRED` 或 `400 FILE_REFERENCE_INVALID`。客户端按 [file references](https://core.telegram.org/api/file-references) 记住文件来源（消息、贴纸集等），重新拉取那条来源，换上新的字节。两条错误按同样方式处理（[Uploading and Downloading Files](https://core.telegram.org/api/files)）。

本仓：

| 证据 | 看到的行为 |
|---|---|
| `proto/schema/telegram_api_layer232.tl` | `document#8fd4c4d8`、`photo#fb197a65`、`inputDocument#1abfb575`、`inputPhoto#3bb3b94a`、`inputDocumentFileLocation#bad07584`、`inputPhotoFileLocation#40181ffe` 都带 `file_reference:bytes` |
| `proto/mtproto/rpc_error_codes.go` | 定义了 `ErrFileReferenceExpired`、`ErrFileReferenceInvalid`、`ErrFileReferenceEmpty`。全仓没有 handler 返回它们 |
| `dfs.uploadPhotoFileV2`、`dfs.uploadDocumentFileV2`、`app/service/media/internal/dao/document.go`、`photo.go` | 新建 document / photo 时 `FileReference` 写成空字节或 `nil` |
| `FilesCore.UploadGetFile`、`DfsCore.DfsDownloadFile` | 下载按 location 的 id 和对象键读字节，不读、不校验 `file_reference` |
| `upload.getFile_retention_test.go` | 保留策略清掉的媒体返回 `FILE_ID_INVALID`。`UploadGetFile` 的注释写明不用 `FILE_REFERENCE_*`，避免客户端反复重拉消息 |

未做：不签发会过期的 `file_reference`，下载也不因这段字节过期或无效而失败。客户端因此不会走到「重拉消息再重试」这条官方刷新路径。

## 2. 分块并行上传 / 断点续传 — partial

官方（[Uploading and Downloading Files](https://core.telegram.org/api/files)）：

- `upload.saveFilePart#b304a621`：文件不大于 10MB。
- `upload.saveBigFilePart#de7b673d`：更大的文件，带 `file_total_parts`。
- 分块大小须能整除 512KB，单块不超过 512KB；最后一块可以更短。
- 客户端用多条连接并行保存不同的 `file_part`。服务端把每块放在临时位置，保留几分钟到几小时。
- 收尾时缺块返回 `FILE_PART_X_MISSING`，客户端补传这一块。没有「列出已保存分块」的 RPC。

本仓两条 RPC 都有路由和 handler，矩阵里也是 `implemented`：

| 证据 | 看到的行为 |
|---|---|
| `FilesCore.UploadSaveFilePart`、`FilesCore.UploadSaveBigFilePart` | 转到 `DfsCore.DfsWriteFilePartData` |
| `Dao.WriteFilePartData`（`app/service/dfs/internal/dao/ssdb.go`） | 每块是 SSDB 哈希 `file_{owner}_{fileId}` 的一个 field，field 名是分块序号。不同序号互不覆盖，写入本身不要求按顺序 |
| `ssdbExpire`（`app/service/dfs/internal/dao/vars.go`） | 每次写入把这个 key 的过期设为 3 小时。3 小时内同一 `file_id` 的已写入块还在 |
| `DfsFileInfo.ToArgs` | 分块 0、分块 1、最后一块各自只 HMSET 非零字段，并行写元数据时不会把另一块的长度抹掉 |
| `model.CheckFilePart`、`model.CheckFilePartSize` | 写了 512KB、`FILE_PART_TOO_BIG`、`FILE_PART_SIZE_INVALID` 的检查。`DfsWriteFilePartData` 不调用它们。写入路径只拒绝空块（`FILE_PART_EMPTY`） |
| `DfsUploadPhotoFileV2`、`DfsUploadDocumentFileV2` | 用 `inputFile.parts` 去 `SSDBReader.ReadAll`。读不到的 field 只要 HGET 不报错，就当成空字节拼进去，不返回 `FILE_PART_X_MISSING` |
| `validateEncryptedSource`、`dfs.uploadEncryptedFileV2_handler_test.go` | 加密文件会在块数、首块长度对不上时返回 `FILE_PART_0_MISSING` 或 `FILE_PARTS_INVALID`。普通照片和文档不走这里 |

[ADR-0031](https://github.com/tellomi/tellomi/blob/main/docs/adr/0031-resumable-media-upload.md)（2026-09-10）把断点续传记成长期方向，实现未排期。它记录的旧路径是 `PUT /media/{id}`：`serveMedia` → `StoreObject` → 对整个 body `io.ReadAll`，再对整段明文 `EncryptV2`；`media_uploads` 没有 `uploaded_offset` / `uploaded_ranges`。那条路径不是续传，重试从 0 开始。它的长期目标是一条显式上传会话：资产身份、预期大小、chunk/range、已确认的 offset/ranges、校验和、过期、续传、收尾。

那条旧的 HTTP 媒体路径不是本盘点覆盖的 Teamgram MTProto 上传路径。当前 `upload.saveFilePart` / `upload.saveBigFilePart`（SSDB 分块、3 小时 TTL、乱序写入）多于 ADR-0031 所记的「当前没有」，但仍不是它的上传会话目标。对照官方 Telegram 分块规则，状态仍是 partial：写入路径不调用 `CheckFilePart` / `CheckFilePartSize`；普通照片和文档收尾不返回 `FILE_PART_X_MISSING`。未做的还有：没有服务端的已保存分块清单（官方同样没有这条 RPC）；并行是客户端多连接的事，本仓没有服务端的并行队列上限，也没有把并行收口成一次可恢复的上传会话。

## 3. CDN DC — partial

官方（[Encrypted CDNs](https://core.telegram.org/cdn)）：CDN 是另一套数据中心。`help.getConfig` 的 `dcOption` 带 `cdn:flags.3?true`。客户端连到那个 DC，只用 `upload.getCdnFile`、`initConnection`、`invokeWithLayer`。密钥来自 `help.getCdnConfig`。主 DC 负责 `upload.reuploadCdnFile`、`upload.getCdnFileHashes`，并在 `upload.getFile` 里返回 `upload.fileCdnRedirect`。CDN 上的字节是 AES-256-CTR 密文，哈希是密文的 SHA-256。

本仓把「主 DC 上的 CDN 下载 / 哈希 / 重传」和「单独的 CDN DC」分开之后，前者有代码，后者没有。

| 证据 | 看到的行为 |
|---|---|
| `FilesCore.UploadGetCdnFile`、`UploadGetCdnFileHashes`、`UploadReuploadCdnFile`、`HelpGetCdnConfig` | 都在 files bff 里，和主 DC 的 `upload.getFile` 同一进程。注释写明这是 master DC |
| `app/bff/files/cdn/cdn.go` | 进程内缓存。AES-256-CTR，密文按 128 KiB 做 SHA-256。单文件不超过 8 MiB，最多 32 个，TTL 1 小时。`upload.getFile` 在客户端带 `cdn_supported` 且缓存装得下时可以返回 `upload.fileCdnRedirect` |
| `helpdesk.PublicKeys` | 读 `TELLOMI_CDN_PUBLIC_KEYS`。空则 `help.getCdnConfig` 的 `public_keys` 为空，三条 CDN 方法返回 `FILE_TOKEN_INVALID`，客户端继续走 `upload.getFile` |
| `cdn_handlers_test.go`、`upload.getFile_cdn_test.go`、`app/bff/files/cdn/cdn_test.go` | 覆盖未发布时的 `FILE_TOKEN_INVALID`，以及发布密钥后的重定向 / 重传 |
| `pkg/dcoptions.Parse`、`help.getConfig_dc_test.go`、[DC_ADDRESSES.md](ops/DC_ADDRESSES.md) | `TELLOMI_DC_OPTIONS` 只接受 `dcId:ipv4:port`。每个 `dcOption` 的 `cdn` 固定为 false。生产列表是 DC 1 的 `8.137.39.82:10443` 和 `:5222`，没有第二条带 `cdn` 的 DC |

未做：没有在 `dc_options` 里广告一个 `cdn:true` 的独立 DC，也没有只接受 `upload.getCdnFile` 的另一套进程。密钥即便配上，密文仍由当前这个 files 进程在内存里现加密，不是发到另一个 CDN DC。

## 4. 按需缩略图尺寸 — partial

官方：`inputPhotoFileLocation#40181ffe` 和 `inputDocumentFileLocation#bad07584` 用 `thumb_size` 选择 `PhotoSize` / `VideoSize` 的 `type`。图片在上传时带上一组缩小尺寸（`s` / `m` / `x` / `y` / `w` 等）。`i` 是嵌在构造器里的 stripped 缩略图，不是按下载请求现算的对象（[files](https://core.telegram.org/api/files)）。

本仓在上传时生成并保存尺寸，下载只按已保存的键去读。客户端要一个从未存过的 `thumb_size` 时，不会现做一档。

| 证据 | 看到的行为 |
|---|---|
| `proto/mtproto/image_thumbnail_types.go` 的 `ReSizeInfoPhotoList` | 照片阶梯是 `s` 90、`m` 320、`x` 800、`y` 1280、`w` 2560，不超过原图边长 |
| `imaging.ReSizeImage`、`imaging/resize_test.go` | JPEG / PNG / WebP 走 govips；GIF / TIFF / BMP 走旧解码。测试要求只出现客户端认识的 `s` / `m` / `x` / `y` / `w` |
| `DfsUploadPhotoFileV2` | 上传时把每一档写成对象，并记入 `media_photo_paths`。命中去重时沿用第一份的键，不重写 |
| `DfsUploadDocumentFileV2` | 当文件的图片只做 inline 的 `i`（stripped）和一档 `m`（320）。不是整条照片阶梯 |
| `Dao.PhotoObjectPath`、`DfsDownloadFile` | `thumb_size` 用来拼 `{type}/{id}.dat`，或读 `media_photo_paths`。对象不存在时打日志并返回空字节，不调用 `ReSizeImage` |
| `dedup.go` 的 `storedPhotoPath` | `i` / `j` 和 cached 尺寸没有对象键，字节在构造器里 |

未做：`upload.getFile` 不会为一档从未写入的 `thumb_size` 现生成图片。请求未存储的尺寸得到空字节，而不是新的一档 `PhotoSize`。

## 相关

- 总仓 ADR-0082 阶段 P3：[`docs/adr/0082-media-dedup-telegram-baseline.md`](https://github.com/tellomi/tellomi/blob/main/docs/adr/0082-media-dedup-telegram-baseline.md)
- 总仓 ADR-0031：[`docs/adr/0031-resumable-media-upload.md`](https://github.com/tellomi/tellomi/blob/main/docs/adr/0031-resumable-media-upload.md)
- 本仓 [media-dedup.md](media-dedup.md)、[adr/0083-server-tech-roadmap-media-obs.md](adr/0083-server-tech-roadmap-media-obs.md)
- 官方：[file references](https://core.telegram.org/api/file-references)、[files](https://core.telegram.org/api/files)、[CDN](https://core.telegram.org/cdn)

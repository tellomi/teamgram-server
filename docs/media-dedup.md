# 媒体去重现状

> 2026-11-05 对照本仓代码记下的现状。本文件不改上传行为。总仓决策见 [`docs/adr/0082-media-dedup-telegram-baseline.md`](https://github.com/tellomi/tellomi/blob/main/docs/adr/0082-media-dedup-telegram-baseline.md)。对象键布局见 [storage.md](storage.md)。保留策略见 [media-retention.md](media-retention.md)。

服务端**没有按内容 hash 去重**。同一文件上传 N 次就存 N 份对象。

## 上传

`dfs.uploadDocumentFileV2`（`app/service/dfs/internal/core/dfs.uploadDocumentFileV2_handler.go`）和 `dfs.uploadPhotoFileV2`（`dfs.uploadPhotoFileV2_handler.go`）每次都 `IDGenClient2.NextId()` 生成新 id，按 `<id>.dat` 写入对象存储。没有按 sha256 或 md5 查已有对象再复用的路径。

`documents` 表没有 sha256 / md5 列，也没有按内容 hash 的唯一索引。`DocumentsDO`（`app/service/media/internal/dal/dataobject/documents_do.go`）的字段是 id、document_id、access_hash、路径、大小、mime 等，不含 hash。

`md5_checksum` 只做完整性校验，不用于复用：

- 图片路径（`dfs.uploadPhotoFileV2` 等）在客户端带了 checksum 且不符时返回 `MD5_CHECKSUM_INVALID`。
- 加密文件（`dfs.uploadEncryptedFileV2`）同样比对 md5，不符则 `MD5_CHECKSUM_INVALID`。`encrypted_files.md5_checksum` 是该校验的存储，不是去重键。

## 封锁桩

- `messages.getDocumentByHash`（`app/bff/files/internal/core/messages.getDocumentByHash_handler.go`）：上游 teamgram enterprise 封锁桩。日志写明需要 teamgram 许可证，然后返回 `documentEmpty`，不按 sha256 + size + mime 查找已有 Document。
- `upload.getFileHashes`（`upload.getFileHashes_handler.go`）：同一类封锁桩，直接返回 `ENTERPRISE_IS_BLOCKED`。

`upload.getCdnFileHashes` 与 `upload.reuploadCdnFile` 已实现，只做 CDN 分块 SHA-256 下载校验，不参与上传去重。

## 现有复用

唯一会复用已有对象的路径是再次发送**同一个** `InputDocument` 或 `InputPhoto`（同一 `id` + `access_hash` + `file_reference`）。`messages.uploadMedia` 对 `inputMediaDocument` 走 `MediaGetDocument`，引用已有记录，不重新上传。换一个用户、重新选文件再发，会走出新 id 的上传路径。

## 待办

实现留到后续 PR，不在本笔记里改 media 代码。顺序：

1. **P1** 内容寻址存储。上传合并后计算 sha256（加 size）。新增 `media_blobs(sha256, size, file_path, ref_count, …)`，或在 `documents` / `photo_sizes` 加 hash 列和索引。命中则新的 document / photo 记录指向已有对象，不再写对象存储。`access_hash` 与 `file_reference` 仍按每条记录独立，共享对象不泄漏权限。
2. **P1** 实现 `messages.getDocumentByHash`（换掉封锁桩，对照官方 TL），让客户端在发送贴纸、GIF、已知文件前按 hash 取已有 Document。
3. **P2** 实现 `upload.getFileHashes`，与 CDN 分块 hash 口径统一。
4. **P2** 生命周期跟 [media-retention.md](media-retention.md) 对齐：共享对象按引用计数删除，保留任务不能删掉仍被引用的 blob。
5. **P3** 其余 Telegram 公开能力（file_reference 刷新、分块并行上传与断点续传、CDN DC、按需缩略图）逐项列入 `docs/api-coverage.md`。

验收：同一视频两个账号各发一次，对象存储只多一份对象；两条消息都能下载；删掉其中一条不影响另一条；e2e 覆盖，并更新 api-coverage。

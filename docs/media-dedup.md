# 媒体去重

> P1（ADR-0082）已在服务端落地：上传合并后按 sha256 + size 复用已有对象。总仓决策见 [`docs/adr/0082-media-dedup-telegram-baseline.md`](https://github.com/tellomi/tellomi/blob/main/docs/adr/0082-media-dedup-telegram-baseline.md)。对象键布局见 [storage.md](storage.md)。保留策略见 [media-retention.md](media-retention.md)。

同一份字节再上传不会再写第二份对象。新的 document / photo 行仍有自己的 `id`、`access_hash` 和 `file_reference`。命中 hash 不是授权。

## 存储

迁移 `teamgramd/deploy/sql/migrate-20261110-media-dedup.sql`：

| 表 | 作用 |
|---|---|
| `media_blobs` | 唯一键 `(storage_class, sha256, file_size)` → 对象键、`ref_count`、第一份上传的 id |
| `media_blob_refs` | 每条 document / photo 指向一个 blob |
| `media_photo_paths` | 复用的图片各档尺寸指向已有对象键 |
| `documents.sha256` / `blob_id` | `messages.getDocumentByHash` 的查找列 |
| `photos.sha256` / `blob_id` | 同一套引用，图片不走 getDocumentByHash |

dfs 和 media 都要能连到这块 MySQL（`dfs.yaml` 的 `Mysql`，与 media 同一个库）。DSN 为空时保持旧行为：每次上传写 `<id>.dat`。

文档（含 `video/mp4`、GIF）在 `documents/<第一份 id>.dat`。图片原图在 `photos/0/<第一份 id>.dat`，衍生尺寸沿用第一份的键。下载先读引用，没有引用时仍按 `<id>.dat`。

`md5_checksum` 仍只做完整性校验，不参与复用：

- 图片路径（`dfs.uploadPhotoFileV2` 等）在客户端带了 checksum 且不符时返回 `MD5_CHECKSUM_INVALID`。
- 加密文件（`dfs.uploadEncryptedFileV2`）同样比对 md5，不符则 `MD5_CHECKSUM_INVALID`。`encrypted_files.md5_checksum` 是该校验的存储，不是去重键。

## 仍是封锁桩

- `upload.getFileHashes`（`upload.getFileHashes_handler.go`）：上游封锁桩，直接返回 `ENTERPRISE_IS_BLOCKED`。P2，本批不实现。

`upload.getCdnFileHashes` 与 `upload.reuploadCdnFile` 已实现，只做 CDN 分块 SHA-256 下载校验，不参与上传去重。

## 现有复用

再次发送同一个 `InputDocument` 或 `InputPhoto`（同一 `id` + `access_hash` + `file_reference`）仍然引用已有记录，不重新上传。重新选文件再发会走上传路径；字节相同则命中 `media_blobs`，新行指向已有对象。

`messages.getDocumentByHash` 按官方 TL：`sha256`（32 字节）+ `size` + `mime_type` 命中则返回已有 Document（含它自己的 `access_hash`），否则 `documentEmpty`。旧构造 `#338e2464` 的 size 是 int，当前 `#b1f2061f` 是 long。这是客户端在发送贴纸、GIF、已知文件前按 hash 取已有 Document 的接口；上传路径不会因为 hash 相同就把别人的 `access_hash` 交给上传者。

删一条消息不会删对象。保留任务扫掉一条 document/photo 时，只要 `media_blobs.ref_count` 还有别的引用，就不删那份对象。

## 待办

1. **P2** 实现 `upload.getFileHashes`，与 CDN 分块 hash 口径统一。
2. **P2** 生命周期跟 [media-retention.md](media-retention.md) 对齐的其余部分：引用归零后的对象删除已经在保留任务里躲开了仍被引用的 blob，完整的保留周期不在 P1。
3. **P3** 其余 Telegram 公开能力（file_reference 刷新、分块并行上传与断点续传、CDN DC、按需缩略图）逐项列入 `docs/api-coverage.md`。

验收：同一视频两个账号各发一次，`documents.file_path` 相同且 `media_blobs` 只有一行；两条都能 `upload.getFile`；删掉其中一条消息后另一条仍能下。e2e：`TestMediaDedupTwoAccounts`。

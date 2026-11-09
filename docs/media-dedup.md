# 媒体去重

> P1（ADR-0082）已在服务端落地：上传合并后按 sha256 + size 复用已有对象。总仓决策见 [`docs/adr/0082-media-dedup-telegram-baseline.md`](https://github.com/tellomi/tellomi/blob/main/docs/adr/0082-media-dedup-telegram-baseline.md)。对象键布局见 [storage.md](storage.md)。保留策略见 [media-retention.md](media-retention.md)。后续路线（波次、观测、明确不做）见 [adr/0083-server-tech-roadmap-media-obs.md](adr/0083-server-tech-roadmap-media-obs.md)。

同一份字节再上传不会再写第二份对象。新的 document / photo 行仍有自己的 `id`、`access_hash` 和 `file_reference`。命中 hash 不是授权。

## 存储

迁移 `teamgramd/deploy/sql/migrate-20261110-media-dedup.sql`：

| 表 | 作用 |
|---|---|
| `media_blobs` | 唯一键 `(storage_class, sha256, file_size)` → 对象键、`ref_count`、第一份上传的 id |
| `media_blob_refs` | 每条 document / photo 指向一个 blob |
| `media_photo_paths` | 复用的图片各档尺寸指向已有对象键 |
| `media_blob_gc` | 最后一条引用放掉之后待删的对象键（`migrate-20261119-media-blob-gc.sql`） |
| `documents.sha256` / `blob_id` | `messages.getDocumentByHash` 的查找列 |
| `photos.sha256` / `blob_id` | 同一套引用，图片不走 getDocumentByHash |

dfs 和 media 都要能连到这块 MySQL（`dfs.yaml` 的 `Mysql`，与 media 同一个库）。DSN 为空时保持旧行为：每次上传写 `<id>.dat`。

文档（含 `video/mp4`、GIF、`image/jpeg` 当文件发、普通文件）在 `documents/<第一份 id>.dat`。图片原图在 `photos/0/<第一份 id>.dat`。第一份上传把各档尺寸写进 `media_photo_paths`；后来的图片和文档缩略图沿用这些键，不再写第二份对象。下载先读引用。没有引用时，文档仍按 `<id>.dat`，图片仍按 `<type>/<id>.dat`。

小 GIF（小于 10KB）按收到的 `image/gif` 字节取 hash。更大的 GIF 先转成 mp4，hash 的是转码后的字节，所以只有转码结果逐字节相同才会复用。

`md5_checksum` 仍只做完整性校验，不参与复用：

- 图片路径（`dfs.uploadPhotoFileV2` 等）在客户端带了 checksum 且不符时返回 `MD5_CHECKSUM_INVALID`。
- 加密文件（`dfs.uploadEncryptedFileV2`）同样比对 md5，不符则 `MD5_CHECKSUM_INVALID`。`encrypted_files.md5_checksum` 是该校验的存储，不是去重键。

## 分块校验

`upload.getFileHashes` 按 location 读 `upload.getFile` 会返回的明文，切成 128 KiB 做 SHA-256。`offset` 丢掉起点早于它的分块，和 `upload.getCdnFileHashes` 同一口径。CDN 那条哈希的是密文，不参与上传去重。旧构造 `#c7025931` 的 offset 是 int，当前 `#9156982a` 是 long，两条都走这个 handler。

`upload.getCdnFileHashes` 与 `upload.reuploadCdnFile` 只做 CDN 分块 SHA-256 下载校验。

## 现有复用

再次发送同一个 `InputDocument` 或 `InputPhoto`（同一 `id` + `access_hash` + `file_reference`）仍然引用已有记录，不重新上传。重新选文件再发会走上传路径；字节相同则命中 `media_blobs`，新行指向已有对象。

`messages.getDocumentByHash` 按官方 TL：`sha256`（32 字节）+ `size` + `mime_type`。命中且调用者可以看这份文件时，返回那条 Document（含它自己的 `access_hash`）；否则 `documentEmpty`。可以看，指公开贴纸 / emoji / 反应 / 面板 GIF，或调用者自己的消息箱（含转发）和已保存 GIF 里已经有这份 document。别人的私有文件不会因为 hash 相同就把 `access_hash` 交出来。同一份字节再上传仍是新的 id 和 `access_hash`。旧构造 `#338e2464` 的 size 是 int，当前 `#b1f2061f` 是 long。

`upload.getFile` 和 `upload.getFileHashes` 要求 location 上的 `access_hash` 与照片、文档、加密文件或贴纸目录里签发的值逐位相同。`access_hash=0` 不是通配。`inputPeerPhotoFileLocation` 没有文件 hash：对方必须可见，且 `photo_id` 是这个 peer 的头像。`file_reference` 只在服务端存过非空值时才校验，对不上返回 `FILE_REFERENCE_EXPIRED`。错误 hash 或找不到 id 返回 `FILE_ID_INVALID`。`messages.sendMedia`、`sendMultiMedia`、`editMessage`、`uploadMedia` 里的 `inputMediaPhoto` / `inputMediaDocument`（含贴纸）同样校验，对不上返回 `MEDIA_INVALID`。

环境变量 `TELLOMI_MEDIA_ACCESS_ENFORCE`：默认 `enforce`（拒绝）。设为 `log` 时只打日志并放行，用来在不换镜像的情况下回滚。其它值仍按拒绝处理。

删一条消息不会删对象，也不会放掉 `media_blob_refs`。保留任务扫掉一条 document/photo 时，只要还有别的引用，就不删那份对象。最后一条引用在同一事务里删掉 blob 行（不留下 `ref_count = 0`），并把对象键写入 `media_blob_gc`；对象本身随后删除。删除失败留下孤儿对象和 gc 行，不留下悬空引用。账号清理在搬走对象之前先放掉这条引用；别的引用还在就不搬。

## 待办

1. **P3** 盘点见 [adr-0082-p3-status.md](adr-0082-p3-status.md)。官方 Telegram 分块上传校验（第 2 项：写入时的分块大小，以及收尾时的 `FILE_PART_<n>_MISSING`）已随 #221 落地，squash `3470142c71aa4d3c9d7ada971e15c85cafb3bc0f`。file_reference 刷新、CDN DC、按需缩略图仍未做完。ADR-0031 的显式上传会话仍未排期。生成的 `docs/api-coverage.md` 不记这些缺口。
2. 头像（`uploadProfilePhoto`）、壁纸、主题、铃声仍按新 id 写对象。公开地址和一部分下载位置直接用这个 id，不走 `media_photo_paths`。加密文件不参与去重。

验收：同一视频、同一张图片、同一 GIF、同一普通文件，两个账号各发一次，对应 `documents.file_path` 或图片尺寸键相同，且 `media_blobs` 只有一行；两条都能 `upload.getFile`；删掉其中一条消息后另一条仍能下。图片是两行 `photos`、各自的 `access_hash`。e2e：`TestMediaDedupTwoAccounts`、`TestMediaDedupPhotoGifAndFile`。

上传与 `messages.getDocumentByHash` 的 trace 见 [link-tracking-zh.md](link-tracking-zh.md)。`TELLOMI_OTEL_ENDPOINT` 为空时不安装导出器。span 属性不写 `access_hash`、对象字节或内容 sha256。

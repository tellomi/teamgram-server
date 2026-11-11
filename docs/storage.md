# Object storage

The dfs service stores files in any S3-compatible endpoint. Local Docker runs
MinIO. Production can use Cloudflare R2 with the same variables. Credentials
are expected to reach only existing buckets whose names start with `tellomi-`.
The process never calls `ListBuckets` or `MakeBucket`. If a configured bucket
is missing, startup fails with the bucket name and the variable that names it.

`upload.saveFilePart` and `upload.saveBigFilePart` still land in Redis (the
SSDB cache). The assembled object is written to S3 by the existing dfs upload
handlers. `upload.getFile` reads those objects back.

## Environment variables

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `TELLOMI_S3_ENDPOINT` | yes | yaml `Minio.Endpoint` | API endpoint. R2: `https://<account>.r2.cloudflarestorage.com`. Local MinIO: `http://minio:9000`. |
| `TELLOMI_S3_REGION` | no | `auto` when yaml is also empty | Signing region. **R2 must be `auto`.** Local MinIO in this repo uses `us-east-1`. |
| `TELLOMI_S3_ACCESS_KEY_ID` | yes | yaml | Access key. |
| `TELLOMI_S3_SECRET_ACCESS_KEY` | yes | yaml | Secret key. |
| `TELLOMI_S3_FORCE_PATH_STYLE` | no | `true` | Path-style URLs (`https://endpoint/bucket/key`). Set `false` only for virtual-hosted style. |
| `TELLOMI_S3_USE_SSL` | no | `true` when the endpoint is `https`, otherwise the yaml value | TLS to the API endpoint. |
| `TELLOMI_S3_BUCKET_ATTACHMENTS` | yes for the prefixed layout | — | Private user files: photos, documents, voice notes, story media, encrypted files, and the canonical copy of profile photos. |
| `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` | yes when attachments is set | — | Public media: avatars, sticker and emoji assets, wallpapers. |
| `TELLOMI_S3_BUCKET_BACKUPS` | no | empty | Backup jobs. Unset or empty skips the check and `PutBackup` returns an error. |
| `TELLOMI_S3_PUBLIC_MEDIA_BASE_URL` | no | empty | CDN origin for public objects, for example `https://media.tellomi.app`. |
| `TELLOMI_S3_ATTACHMENTS_BASE_URL` | no | empty | CDN origin for private objects, for example `https://cdn3.tellomi.app`. Callers must still authorize the download. |

Environment values replace the `Minio` block in `dfs.yaml`. Docker Compose
passes them into the `teamgram` service (`docker-compose.yaml`). A non-empty
`TELLOMI_S3_BUCKET_ATTACHMENTS` selects the prefixed layout below. If that
variable is empty, dfs keeps the historical separate buckets
(`documents`, `photos`, `videos`, `encryptedfiles`) with no key prefix.

## Key layout

Teamgram used one bucket per kind of file. Those names are now prefixes inside
the two configured buckets. Create the buckets yourself before starting dfs.

| Logical class | Bucket variable | Object key |
|---|---|---|
| `documents` (files, voice notes, story documents) | `TELLOMI_S3_BUCKET_ATTACHMENTS` | `documents/<id>.dat` |
| `photos` (message photos, story photos, thumbs) | `TELLOMI_S3_BUCKET_ATTACHMENTS` | `photos/<size>/<id>.dat` |
| `videos` (video sizes, including profile-video canonical bytes) | `TELLOMI_S3_BUCKET_ATTACHMENTS` | `videos/<size>/<id>.dat` |
| `encryptedfiles` | `TELLOMI_S3_BUCKET_ATTACHMENTS` | `encryptedfiles/<id>.dat` |
| `avatars` (profile photos and profile videos) | `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` | `avatars/<size>/<id>.dat` |
| `stickers` | `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` | `stickers/<path>` |
| `emoji` | `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` | `emoji/<path>` |
| `wallpapers` | `TELLOMI_S3_BUCKET_PUBLIC_MEDIA` | `wallpapers/<path>` |
| backups | `TELLOMI_S3_BUCKET_BACKUPS` | the path passed to `PutBackup` |

Profile photos and profile videos are written twice: the canonical object
stays on `photos/` or `videos/` so `upload.getFile` does not change, and a
public copy is written under `avatars/`. Wallpapers are the same: the document
and its thumb stay on `documents/` and `photos/`, with a copy under
`wallpapers/`. `upload.getFile` tries the canonical key first and then the
public prefix, so a public-only object still downloads. A catalog sticker
or emoji (`catalog.DocumentAccess` / `PublicDocumentID`, storage class
`stickers` or `emoji`) is read from that public prefix directly, so the
request does not pay for a `documents/` miss first.

A `photo_sizes` row whose object is missing used to make `upload.getFile`
return an empty `upload.file`. Clients treat that as success and retry.
The download tries the recorded key, then `<type>/<id>.dat`, then the
deduped origin's key. A JPEG, PNG, or GIF document is resized with the
same encoder as upload, and only after the standard decoder accepts both
the source and the result. A damaged JPEG is `FILE_ID_INVALID`, not the
original bytes. A video is rebuilt with its own ffmpeg process, not the
shared transcoder; one larger than 32MiB is streamed to a temp file up to
256MiB. Two rebuilds may run at once and share a 512MiB disk budget. The
request context cancels the process. Anything else, including a video
above that cap, returns `FILE_ID_INVALID`. A storage timeout, a full
budget, or a refused read is returned as that error and is not rewritten
to `FILE_ID_INVALID`. Logs count candidates and do not include the object
path or media id. Needs a real ffmpeg binary and a temp directory with
room for the budget. There is no database migration. Do not run
`backfillthumbs -dry-run=false` to accept this change.
Rows already stored without an object are repaired by
`backfillthumbs`, which is dry-run unless `-dry-run=false`:

```
docker exec tellomi-teamgram-1 /app/bin/backfillthumbs -f /app/etc2/dfs.yaml -dry-run
docker exec tellomi-teamgram-1 /app/bin/backfillthumbs -f /app/etc2/dfs.yaml -dry-run=false
```

`FLAG` lines are rows it could not rebuild. It does not delete rows or
objects. Resume a partial walk with `-after` and the last id it prints.

Story media uses the normal photo and document upload, so it lives under
`photos/` and `documents/` in the attachments bucket. Voice notes are
documents (`audio/ogg`) under `documents/`.

There is no automatic copy from the old `documents` / `photos` / `videos` /
`encryptedfiles` buckets. Move those objects under the prefixes above if an
existing volume must be kept.

Public URLs, when the base is set, are
`<TELLOMI_S3_PUBLIC_MEDIA_BASE_URL>/<class>/<path>` (for example
`https://media.tellomi.app/avatars/a/123.dat`). Attachment URLs are
`<TELLOMI_S3_ATTACHMENTS_BASE_URL>/<class>/<path>`.

## Local MinIO

`docker-compose-env.yaml` keeps MinIO behind Compose profile `local-minio`.
A default `up` does not pull or start it (production uses R2). Local buckets:

```bash
docker compose -f docker-compose-env.yaml --profile local-minio up -d
```

`minio-mc` creates:

- `tellomi-attachments`
- `tellomi-public-media`
- `tellomi-backups`

The app does not create them. Defaults in `.env.example` and
`docker-compose.yaml`:

```
TELLOMI_S3_ENDPOINT=http://minio:9000
TELLOMI_S3_REGION=us-east-1
TELLOMI_S3_ACCESS_KEY_ID=minio
TELLOMI_S3_SECRET_ACCESS_KEY=miniostorage
TELLOMI_S3_FORCE_PATH_STYLE=true
TELLOMI_S3_USE_SSL=false
TELLOMI_S3_BUCKET_ATTACHMENTS=tellomi-attachments
TELLOMI_S3_BUCKET_PUBLIC_MEDIA=tellomi-public-media
TELLOMI_S3_BUCKET_BACKUPS=tellomi-backups
```

`us-east-1` is MinIO's signing region. Path-style is on, so the hostname does
not need to be a bucket subdomain. `TELLOMI_S3_USE_SSL=false` matches this
`http://` endpoint. Do not copy that `false` onto an `https://` R2 endpoint:
an explicit false turns TLS off and dfs then misses the objects. Omit the
variable and the `https` scheme turns TLS on. Sticker import uses this same
client, not a separate one.

## Cloudflare R2

Create the buckets in the Cloudflare dashboard (names must already exist and
should use the `tellomi-` prefix). Issue a token that can read and write only
those buckets. Do not grant `ListBuckets` or bucket-create.

```
TELLOMI_S3_ENDPOINT=https://<accountid>.r2.cloudflarestorage.com
TELLOMI_S3_REGION=auto
TELLOMI_S3_ACCESS_KEY_ID=<r2 access key id>
TELLOMI_S3_SECRET_ACCESS_KEY=<r2 secret>
TELLOMI_S3_FORCE_PATH_STYLE=true
TELLOMI_S3_USE_SSL=true
TELLOMI_S3_BUCKET_ATTACHMENTS=tellomi-attachments
TELLOMI_S3_BUCKET_PUBLIC_MEDIA=tellomi-public-media
TELLOMI_S3_BUCKET_BACKUPS=tellomi-backups
TELLOMI_S3_PUBLIC_MEDIA_BASE_URL=https://media.tellomi.app
TELLOMI_S3_ATTACHMENTS_BASE_URL=https://cdn3.tellomi.app
```

`auto` is required. R2 rejects other region names. Path-style (`true`) avoids
virtual-hosted bucket DNS. If `TELLOMI_S3_USE_SSL` is omitted and the endpoint
starts with `https://`, TLS is turned on.

Leave `TELLOMI_S3_BUCKET_BACKUPS` empty when you have no backup bucket. A set
value is checked at startup and must already exist.

The client is `github.com/minio/minio-go/v7` with `Region` set (so it does not
call GetBucketLocation) and `BucketLookup` set to path style. Server-side
encryption headers from MinIO's `encrypt` package are not sent; secret-chat
files are already encrypted by the client.

## Download cache

`upload.getFile` for photo sizes and documents keeps a local LRU of the bytes
it just read. A repeat of the same offset and limit does not call R2 again.
The cache is per process. Caps are bytes. `0` turns that tier off.

```
TELLOMI_DFS_CHUNK_CACHE_MEM=67108864
TELLOMI_DFS_CHUNK_CACHE_DISK=536870912
TELLOMI_DFS_CHUNK_CACHE_DIR=/tmp/tellomi-dfs-chunk-cache
```

Unset, those are 64MiB in memory and 512MiB on disk under the temp directory.
Photo and document objects are immutable for a key, so a hit is not rechecked.
Restart the dfs process after lowering a cap; the next start evicts down to it.

## Content identity

New uploads of the same bytes share one object. dfs hashes the merged file
(sha256 and size) and records it in `media_blobs`. A hit stores no second
object; the new document or photo row points at the first upload's key
(`documents/<id>.dat` or `photos/0/<id>.dat`). `access_hash` stays per row.
See [media-dedup.md](media-dedup.md). Rows written before that migration keep
their own `<id>.dat` key.

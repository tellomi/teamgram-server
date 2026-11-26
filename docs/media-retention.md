# Chat media retention

Chat photos, videos, voice notes and files are removed 7 days after the
original upload. Text is never removed. A subscriber in the chat does not
extend that clock.

## Policy

| Rule | Behaviour |
|---|---|
| Text | Kept forever. The message stays; only the file object goes. |
| Chat media (photos, videos, voice, files) | Removed `TELLOMI_MEDIA_RETENTION_DAYS` (7) days after the **original upload**. Forwarding reuses the same media id, so it does not reset the clock. |
| Subscribers | Do not keep media. After 7 days the file expires the same way whether or not a holder is premium, or is inside the old grace window after `premium_expire_date`. |
| Upgrade | Does not restore a file that was already removed. |
| Exempt | Avatars (photos with a/b/c/d sizes), stickers, custom emoji, GIFs (`documentAttributeAnimated` or `image/gif`), wallpapers/themes, the public-media bucket, and media no message references (link previews, wallpapers). |
| Thumbnails | Kept. Photos keep sizes `s`, `i` (stripped) and `j`; documents keep their thumbs. The chat still shows a blurred preview. |
| Secret-chat files | Not removed. Secret chats keep no server-side participant list, so the sweeper cannot tell who holds them. |

`TELLOMI_MEDIA_RETENTION_GRACE_DAYS` is no longer read. A deployment that
still sets it has no effect.

## What the client sees

`upload.getFile` returns `400 FILE_ID_INVALID` for the full-size object of
expired media (documents without `thumb_size`; photo sizes `0`/`m`/`x`/`y`/`w`;
encrypted files). Thumbnails still download.

- iOS keeps the blurred thumbnail and the download spinner stops.
- Android shows the download button again.
- Neither client shows an "expired" label without a client change.
  `FILE_REFERENCE_*` was not used because it makes clients refetch the
  message and retry forever.

`upload.getCdnFile` tokens are only issued by `upload.getFile`, which checks
first, so expired media never reaches the CDN path.

Deleting a file as soon as the recipient has downloaded it is not part of
this policy.

## Configuration

The sweeper stays off, and dry-run stays on, unless the environment says
otherwise. A deployment that already sets these variables keeps its own
values. Changing the defaults here does not rewrite a live environment.

| Variable | Default | |
|---|---|---|
| `TELLOMI_MEDIA_RETENTION_ENABLED` | `false` | Starts the daily sweeper in the media service. |
| `TELLOMI_MEDIA_RETENTION_DAYS` | `7` | Days after the original upload. |
| `TELLOMI_MEDIA_RETENTION_DRY_RUN` | `true` | Log what would expire; delete nothing. Set to `false` to actually delete. |
| `TELLOMI_MEDIA_RETENTION_BATCH` | `500` | Rows per query. |
| `TELLOMI_MEDIA_RETENTION_MAX_PER_RUN` | `5000` | Cap on items expired per run (0 = none). |
| `TELLOMI_MEDIA_RETENTION_RUN_HOUR` | `4` | Container-local hour of the daily run. |

## Running once

```sh
docker compose ... exec teamgram /app/bin/mediaretention -f /app/etc2/media.yaml            # dry run (default)
docker compose ... exec teamgram /app/bin/mediaretention -f /app/etc2/media.yaml -dry-run=false
```

The report line looks like:

```
media retention DRY_RUN: cutoff=... scanned documents=N photos=N; exempt=N unreferenced=N expire=N ; would_expire documents=N photos=N bytes=N ...
```

## How it works

- `app/service/media/retention`: policy (`Decide`), SQL store, sweeper, and
  the cached `Checker` used by `upload.getFile`.
- Candidates are `documents` / `photos` with `expired_at = 0` and an upload
  time (`date2`, else `created_at`) older than the cutoff.
- Holders come from `messages` through the generated columns
  `media_document_id` and `media_photo_id`
  (`migrate-20261125-media-json-columns.sql`), basic-group members from
  `chat_participants`, and channel members for channels whose
  `channel_messages` carry the media (decoded once per run). Holders only
  answer "does anyone still reference this?". Premium is not loaded.
- Expiry deletes the objects from the attachments bucket and sets
  `expired_at`. Public-media classes are refused by
  `objectstore.Client.Delete`.
- A document or photo that shares a `media_blobs` row does not take the
  object with it while another `media_blob_refs` row remains. The last
  reference deletes the blob row in that same transaction (`ref_count` is
  never left at 0) and inserts the object keys into `media_blob_gc`.
  The object delete happens after the commit. A failed delete leaves the
  gc row and an orphan object; the next live run retries it. Photo
  thumbnails `s`, `i` and `j` stay. A user delete is a different path:
  it wipes the message body and, once nothing else holds the media,
  queues every size and video thumb for deletion. See
  [media-purge.md](media-purge.md).

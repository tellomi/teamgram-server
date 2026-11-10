# Deleting message content and media

A delete removes the copy the client asked to remove. `messages.deleteMessages` and `messages.deleteHistory` with `revoke` unset clear only that user's box. `revoke` also clears the other private-chat box and, for a basic group, the other members' boxes. `just_clear` leaves the service "history cleared" message and wipes the rest of that user's box. `channels.deleteMessages` and `channels.deleteHistory` with `for_everyone` clear the single channel copy. Account deletion clears both sides of private chats and leaves group messages in place. Channel auto-delete (`TELLOMI_MESSAGE_EXPIRY_SWEEP=1`) calls the same channel delete, so the body is wiped there too. Private-chat TTL on this branch still only hides a row until that sweep deletes it.

The row stays, with `deleted=1`, so pts and `updateDeleteMessages` do not change. `message` becomes empty and `message_data` becomes `{}`. A channel row's `message` and `message_data` are cleared the same way.

## Who still holds a file

`message_media_refs` has one row per message box, channel message, or scheduled message that points at a `photo_id` or `document_id`. A forward is another row with the same id. A re-upload is a new id that shares `media_blobs` through `media_blob_refs`.

The object is deleted only when all of these are true:

- no live user message (including saved and pinned) still has the id
- no live channel message, story, or scheduled message has it
- it is not a profile, chat, or channel photo
- it is not a sticker, a saved GIF, saved music, or an emoji status
- `media_external_refs` has no row for it (content-engine writes here when it keeps using a teamgram id)

`ref_count` then drops by one. The R2 object, including every photo size and video thumb, is deleted only when that was the last blob ref. Another document that still shares the hash keeps the object, and `upload.getFile` for the remaining id still works.

A key is also kept when anything serves it without a `media_blob_refs` row. That includes a legacy `documents.file_path` or photo size from before dedup, a catalog sticker or GIF object path, and `media_external_refs`. The worker skips that key. The dry-run line counts it as `kept_legacy`. A document whose blob ref this purge already released is marked expired, so the leftover path is not treated as one of those holders.

Objects in the `crash-reports` class, and any path whose key starts with `crash-reports/`, are also skipped. Those are copies of logs sent to @TellomiBot. Deleting the chat message can still expire the original document id. The crash-report copy stays. See [crash-reports.md](crash-reports.md).

## Queue

The delete enqueues `media_purge_queue` with `available_at = now + grace`. The default grace is 24h (`TELLOMI_MEDIA_PURGE_GRACE`, a Go duration or a number of seconds). The worker re-checks holders after that. A forward that landed in the meantime cancels the row. A failed object delete leaves the row and retries; deleting a missing object is success.

| Variable | Default | |
|---|---|---|
| `TELLOMI_MEDIA_PURGE_ENABLED` | false | Starts the loop in the media service. Deletes enqueue either way. |
| `TELLOMI_MEDIA_PURGE_DRY_RUN` | true | Log the pass and delete nothing. |
| `TELLOMI_MEDIA_PURGE_GRACE` | `24h` | |
| `TELLOMI_MEDIA_PURGE_INTERVAL` | `1m` | |
| `TELLOMI_MEDIA_PURGE_BATCH` | `100` | |

## Enabling object deletion

`docker-compose.yaml` passes these into the `teamgram` container. `.env.example` leaves the worker off and dry-run on. A delete still enqueues. Nothing is removed from R2 or MinIO until a queued row is past its grace and the worker is running live.

Set both on the host `.env` and recreate the container. Leave the grace at 24h.

```
TELLOMI_MEDIA_PURGE_ENABLED=1
TELLOMI_MEDIA_PURGE_DRY_RUN=0
```

`ENABLED=1` with dry-run left at its default only logs. `DRY_RUN=0` without `ENABLED=1` does not start the loop. The one-shot command below is not this switch.

Each due row is checked again before the object is removed: live messages, pins, channel posts, stories, scheduled messages, avatars, stickers, saved GIFs, `media_external_refs`, and a legacy `file_path` that has no blob ref. A holder cancels that row. The delete uses the same `TELLOMI_S3_*` client as uploads, so a production endpoint of `https://<accountid>.r2.cloudflarestorage.com` removes the R2 object (bucket `TELLOMI_S3_BUCKET_ATTACHMENTS`, key `<class>/<path>`). A missing object is success. A failed delete leaves the row and retries. Avatars, stickers, emoji, and wallpapers are not removed on this path.

The worker log line is `media purge DRY_RUN:` or `media purge LIVE:` and counts due, held, released, objects, bytes, shared blobs, and errors. It does not log message text or file bytes.

## One-off report

This does not delete objects. It backfills `message_media_refs` from live messages and prints what historical deleted rows still point at:

```sh
docker compose ... exec teamgram /app/bin/mediapurge -f /app/etc2/media.yaml
docker compose ... exec teamgram /app/bin/mediapurge -f /app/etc2/media.yaml -backfill=false
```

```
media purge DRY_RUN: backfilled=N live_refs=N deleted_messages_with_media=N candidates=N would_free_blobs=N would_free_objects=N would_free_bytes=N photos=N documents=N kept_message=N kept_pinned=N kept_channel=N kept_avatar=N kept_story=N kept_sticker=N kept_saved=N kept_scheduled=N kept_external=N kept_shared_blob=N kept_legacy=N
```

`-dry-run=false` processes the queue. It still does not delete the historical candidates above; those stay until an operator enqueues them after a backup. `-grace=0` processes ids whose grace has not elapsed, after the same holder check.

Migration: `teamgramd/deploy/sql/migrate-20261120-media-purge.sql`. New tables only.

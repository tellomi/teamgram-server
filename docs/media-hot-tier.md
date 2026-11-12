# Chat media hot tier

Default **off** (`TELLOMI_MEDIA_HOT_TIER_ENABLED`). When off, dfs writes
every object to the S3-compatible endpoint immediately, which is today's
behaviour.

When on, new chat media (photos, videos, documents including voice,
encrypted files) is written first to a directory on the data disk. Reads
try that directory, then fall back to the object store. After the current
retention window the migrator copies the file to the same bucket and key
it would have used before, checks size and sha256, and only then deletes
the local copy. Existing objects that are already in the bucket are not
copied back to disk.

The 90-day free-media deleter and the 24-hour unreferenced purge are
unchanged. Both go through `RemoveClassObject` / `MoveToTrash`, which also
delete a local copy when one exists.

## 105 must not use the public network

If the flag is on, `TELLOMI_S3_HTTP_PROXY` is required (production:
`http://172.19.44.222:3128`). A missing or invalid proxy is a startup
error. The client is not created, buckets are not probed, and the
process never dials the R2 endpoint directly. CONNECT failure has no
direct-dial fallback (same as the #322 proxy itself).

## Settings

```
TELLOMI_MEDIA_HOT_TIER_ENABLED=true
TELLOMI_MEDIA_HOT_TIER_DIR=/data/tellomi/media-hot
TELLOMI_S3_HTTP_PROXY=http://172.19.44.222:3128
TELLOMI_MEDIA_HOT_TIER_MAX_AGE=168h
TELLOMI_MEDIA_HOT_TIER_USED_PCT_7D=60
TELLOMI_MEDIA_HOT_TIER_USED_PCT_1D=75
TELLOMI_MEDIA_HOT_TIER_USED_PCT_12H=85
TELLOMI_MEDIA_HOT_TIER_USED_PCT_1H=90
TELLOMI_MEDIA_HOT_TIER_USED_PCT_EMERGENCY=90
TELLOMI_MEDIA_HOT_TIER_INTERVAL=1m
TELLOMI_MEDIA_HOT_TIER_QUIET=2m
```

Used-percent is taken from the filesystem that holds the hot directory
(the data disk, not the root disk). Crossing each threshold shortens
retention from 7 days to 1 day, 12 hours, 1 hour, then immediate
migration. Files still being written, or quieter than `QUIET`, are
skipped. The media and mediapurge processes must see the same directory
and proxy so a sweep deletes the local copy as well.

Do not shorten `TELLOMI_MEDIA_RETENTION_DAYS` to try to free the data
disk; that deletes the object from R2.

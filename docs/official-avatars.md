# Official account avatars

The official accounts seeded by
`teamgramd/deploy/sql/migrate-20261105-official-accounts.sql` (users 777000
to 777005) have no auth key, so nobody can call `photos.uploadProfilePhoto`
for them. `seedofficialavatars` gives them profile photos through the same
services that call uses:

1. `dfs.writeFilePartData` stores the embedded bytes as upload parts.
2. `media.uploadProfilePhotoFile` builds the photo sizes (and, for 777001,
   the 800x800 `u` video size) in object storage and the media tables.
3. `user.updateProfilePhoto` sets `users.photo_id` and `user_profile_photos`
   through the cached user DAO, so no manual Redis cleanup is needed.

| User | Account | Asset |
|---|---|---|
| 777000 | Tellomi (@admin) | Tellomi logo on #17171A |
| 777001 | Tellomi AI (@tellomi) | animated avatar (mp4) + static JPEG |
| 777002 | Tellomi (@tellomi_official) | Tellomi logo on #17171A |
| 777003 | Tellomi Team | logo on #2F6BFF |
| 777004 | Tellomi Support | logo on #12A37F |
| 777005 | 发现 (@faxian) | logo on #F08A24 |

Logos are rendered at 640x640 from `docs/brand/logo.svg` in the
tellomi/tellomi repository. Assets live in
`app/bff/userchannelprofiles/officialavatars/assets` and are embedded in the
binary.

## Running

Inside the teamgram container (host network, etcd on 127.0.0.1:2379), with
the services up:

```bash
docker exec tellomi-teamgram-1 /app/bin/seedofficialavatars
# replace one avatar after changing its asset
docker exec tellomi-teamgram-1 /app/bin/seedofficialavatars -force -users 777001
```

Accounts that already have a photo are skipped unless `-force` is set, so the
command is safe to rerun. Clients that cached the old (empty) user show the
photo after the next user refresh (reopen the chat or restart the app).

## Official channels

The official channels 1073800001–1073800004 (官方公告 @news, 更新日志 @updates,
安全中心 @security, 开发者 @dev) get their photos with `-channels`. The upload is
owned by 777002 (the channel creator) and the photo is applied through the
MySQL channel service's `ApplyPhoto`, the same code `channels.editPhoto` runs,
so each channel gets a "photo updated" service message and a new version.

```sh
docker exec -e TEAMGRAM_MYSQL_DSN='root:...@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai' \
  tellomi-teamgram-1 /app/bin/seedofficialavatars -users none -channels all
# one channel, replacing its photo:
... /app/bin/seedofficialavatars -users none -channels 1073800003 -force
```

Run one seeder at a time; channels are processed sequentially.

## Stripped thumbs for photos already stored

Profile photos uploaded before the inline thumb was saved have no size `i`.
Clients then keep the placeholder until `upload.getFile` returns, and a
cached empty download never shows the JPEG. `backfillstripped` reads the
stored JPEG (size `a`, then `b` / `c` / `d`, then the message ladder) and
inserts `photo_sizes` size `i`. It does not delete or replace objects.
A photo that already has those bytes is skipped, so the command can be
run again. Pass `-after` with the last id it prints to resume.

The user and chat caches keep the old photo for about seven days. Pass
`-redis` so those entries (`photo#`, `user_data.2#`, `chat#`) are dropped.
That is not `FLUSHALL`.

Dry-run is the default. Inside the teamgram container, after this image
is deployed:

```bash
docker exec tellomi-teamgram-1 /app/bin/backfillstripped -f /app/etc2/dfs.yaml -dry-run
docker exec tellomi-teamgram-1 /app/bin/backfillstripped \
  -f /app/etc2/dfs.yaml -dry-run=false -redis redis:6379 -batch 50
# stop after 500 photos, then continue
docker exec tellomi-teamgram-1 /app/bin/backfillstripped \
  -f /app/etc2/dfs.yaml -dry-run=false -redis redis:6379 -max 500
docker exec tellomi-teamgram-1 /app/bin/backfillstripped \
  -f /app/etc2/dfs.yaml -dry-run=false -redis redis:6379 -after <last>
```

The container starts dfs from `/app/etc2/dfs.yaml`
(`teamgramd/bin/runall-docker.sh` passes `../etc2/dfs.yaml`, and the
working directory is `/app/bin`). That file uses MySQL at `mysql:3306`
and Redis at `redis:6379`. The yaml under `/app/etc` is the host-network
file and points MySQL and Redis at loopback. Inside this container that
loopback is not Redis and not MySQL. The binary default for `-f` is
`/app/etc2/dfs.yaml`. `-redis` stays empty unless it is set, so a write
does not drop `photo#`, `user_data.2#`, or `chat#` until
`-redis redis:6379` is passed.

The dfs yaml supplies the MySQL DSN and object storage. `TELLOMI_S3_*`
overrides those settings the same way the dfs process does. Photos come
from `users.photo_id`, `user_profile_photos`, `chats.photo_id`, and
`channels.photo_id`. A missing `channels` table is ignored.

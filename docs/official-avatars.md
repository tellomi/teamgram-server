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

# Security operations

Locations only. This file does not contain secret values. Repo defaults below are for local Docker. Chengdu production rotated them on 2026-11-03.

## MTProto RSA

The production MTProto RSA key was rotated on 2026-11-03.

| Key | Fingerprint | Where |
|---|---|---|
| Current production key | `4381149949997195609` | Private key on the Chengdu host, not in git. gnetway loads it first when `TELLOMI_MTPROTO_EXTRA_KEY_FILE` and `TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT` are both set. |
| Upstream Teamgram default | `12240908862933197005` | `teamgramd/bin/server_pkcs1.key` and `proto/mtproto/crypto/test_pkcs1.key`. Still listed in `teamgramd/etc2/gnetway.yaml` `RSAKey`. |

The fingerprint is the lower 64 bits (last 8 bytes, little-endian) of SHA1 of the TL string serialization of modulus `n` and exponent `e`. gnetway computes that at startup and exits if it does not match the configured decimal fingerprint.

The upstream key is public. It must not stay in production long-term. It remains in `RSAKey` only so clients that still carry that public key can finish the handshake. After clients with the new public key are widespread, remove it from `gnetway.yaml` `RSAKey`.

`docker-compose.yaml` passes the two `TELLOMI_MTPROTO_EXTRA_KEY_*` variables into `teamgram`. Empty means the yaml list alone. On Chengdu the private key directory is mounted read-only at `/etc/tellomi/mtproto/` and the file variable points inside that mount.

## Database, MinIO, and Grafana passwords

MySQL `root`, the MySQL user `teamgram`, the MinIO root user, and the Grafana admin user were rotated on 2026-11-03.

These repo defaults are dev-only:

| What | Dev default |
|---|---|
| MySQL application user | `teamgram` / `teamgram` |
| MySQL root | `root` |
| MinIO root user | `minio` |
| Grafana admin | `admin` |

`teamgramd/etc2/*.yaml` still contains `teamgram:teamgram@tcp(mysql:3306)`. When `TEAMGRAM_MYSQL_PASSWORD` is non-empty, `teamgramd/docker/entrypoint.sh` replaces the password in that DSN. `docker-compose.yaml` sets `TEAMGRAM_MYSQL_PASSWORD` from `MYSQL_PASSWORD`, default `teamgram`. If the password is anything else and `teamgram:teamgram@` is still in `/app/etc2` after the rewrite, the container exits. The dev password is `teamgram`, so that string remaining is the local database credential, not a failed rewrite.

Schema migration uses `MYSQL_ROOT_PASSWORD` (dev default `root`) and is separate from the application DSN.

## Startup logs

Each service logs its config at startup (`logx.Infov`). That copy goes through `pkg/redact` first. Passwords inside MySQL DSNs are replaced with `***` (the user, host, and database stay). Fields whose names contain `Secret`, `Password`, `Pass`, or `Key` are replaced with `***`. That covers Redis `Pass`, MinIO and S3 secret keys, and private keys. The process still uses the real config.

Logs written before this change on Chengdu can still contain the database password. That includes `docker logs` for the teamgram containers and the files under `/app/logs`. Purge or rotate those logs after this change is deployed. Do not copy them into tickets or chat.

## Where secrets live

Do not copy values into git, issues, or chat.

Chengdu host:

| Path | What |
|---|---|
| `/opt/tellomi/teamgram-server/.env` | Compose environment for the running stack. |
| `/opt/tellomi/secrets/mtproto/` | MTProto private keys. Mounted read-only at `/etc/tellomi/mtproto/`. |
| `/opt/tellomi/runtime/` | Host entrypoint wrapper `teamgram-dsn-entrypoint.sh` and `pwrotate-rollback.sh`. |
| `/opt/tellomi/backups/` | Database dumps, including `pre-pwrotate-20261103-194329/`. |

The team key vault folder is `成都服务器-运行时/`. Its index is `索引.md` (`db-minio-passwords.md`, `mtproto-rsa-2026-11/`).

Media is on Cloudflare R2, not the local MinIO. MinIO in `docker-compose-env.yaml` is the `local-minio` profile for a workstation. Do not start that profile on Chengdu.

The image now performs the DSN rewrite and the extra RSA key load. The host wrapper under `/opt/tellomi/runtime/` is the previous way those two were applied. Leave that directory in place; it still holds `pwrotate-rollback.sh`.

## Deploy

The Publish images workflow (`.github/workflows/docker-acr.yml`) builds images and pushes them to Aliyun ACR. On the Chengdu host, after a database backup:

```bash
/opt/tellomi/deploy-pull.sh <sha7>
```

`<sha7>` is the 7-character commit SHA already published by that workflow. The script in this repo is [`deploy-pull.sh`](../deploy-pull.sh). It checks out that commit, pulls the image, runs `mysql-migrate`, and recreates `teamgram` (and `asr` when the commit defines it). It does not build on the server.

`deploy-pull.sh` runs `git fetch` against the public GitHub remote. If this repository becomes private, put a read-only deploy key on the host before the next fetch. Do not point the fetch at a mirror.

## Root disk

Deploying `a6e919c` filled the Chengdu root disk with old images and handshakes failed for about five minutes. `deploy-pull.sh` now refuses to touch the checkout when `/` is short of free space. The cleanup and cron files live in git; the host copies are installed by hand.

```bash
install -m 755 deploy-pull.sh /opt/tellomi/deploy-pull.sh
install -m 755 scripts/ops/tellomi-image-cleanup.sh /usr/local/bin/tellomi-image-cleanup.sh
install -m 755 scripts/ops/tellomi-disk-watch.sh /usr/local/bin/tellomi-disk-watch.sh
install -m 644 scripts/ops/tellomi-disk-watch.cron /etc/cron.d/tellomi-disk-watch
```

`git checkout` does not update `/opt/tellomi/deploy-pull.sh` or `/usr/local/bin`. Copy them again after this repo changes.

Retention, per `tellomi-image-cleanup.sh`. Only 7-hex tags of `*/tellomi/teamgram` and `*/tellomi/asr` are candidates. Kept:

- the tag each container was created from (running or stopped);
- the sha passed to the deploy (`--keep`), which is this deploy's tag;
- the two newest other `teamgram` tags (`TELLOMI_KEEP_RECENT`, default 2). The same sha set is kept for `asr`.

Newest is image creation time, then the local tag time (`Metadata.LastTagTime`, set when the tag was pulled), then the git commit time in `TELLOMI_DEPLOY_DIR`, then the tag string. Creation time alone is not a stable order: two images with the same `Created` could sort the rollback tag behind an older one and delete it. After that, `docker image prune -f` removes dangling layers only. Volumes, containers, `/opt/tellomi/backups`, and other images are not touched.

Thresholds:

- Before checkout, if `df -BG` reports less than `TELLOMI_MIN_FREE_GB` (default 10) GiB free on `/`, the script runs cleanup. If free space is still short, it exits 3 and does not change the checkout, `.env`, or containers.
- `TELLOMI_DEPLOY_CHECK_ONLY=1` stops before checkout. When free space is already under the threshold, cleanup is `--dry-run` and the script still exits 3, because a dry-run does not free space. When free space is already enough, it prints `disk: ok` and `CHECK_ONLY: stop before deploy`.
- `/etc/cron.d/tellomi-disk-watch` runs every 10 minutes. Above `TELLOMI_DISK_WARN_PCT` (default 85) percent used on `/`, it logs a warning and runs the same cleanup. 85 percent does not fire; 86 percent does.

A failed post-deploy cleanup prints a warning and does not roll the deploy back. `mysql-migrate` still has to exit 0 before `teamgram` is recreated, and the GeoIP file and `docker-compose.tellomi.yaml` are still required.

## Database backups

MySQL, etcd, and Redis are dumped at 03:15, encrypted with age, and uploaded to Cloudflare R2 bucket `tellomi-prod-db-backups`. Local files under `/opt/tellomi/backups/daily` are kept 90 days. R2 lifecycle deletes the remote objects; the script does not. Redis is `BGSAVE` plus a copy of `dump.rdb` and `appendonlydir`. The script never runs `FLUSHALL`. The age private key stays offline. Setup, the bucket token, Bucket Lock, and the rclone 1.60.1 flags are in [ops/BACKUP.md](ops/BACKUP.md).

```bash
install -m 755 scripts/ops/tellomi-backup-common.sh /usr/local/bin/tellomi-backup-common.sh
install -m 755 scripts/ops/tellomi-db-backup.sh /usr/local/bin/tellomi-db-backup.sh
install -m 644 scripts/ops/tellomi-db-backup.cron /etc/cron.d/tellomi-db-backup
install -m 755 scripts/ops/tellomi-backup-verify.sh /usr/local/bin/tellomi-backup-verify.sh
install -m 644 scripts/ops/tellomi-backup-verify.cron /etc/cron.d/tellomi-backup-verify
```

`git checkout` does not update `/usr/local/bin`. Copy them again after this repo changes. Leave `TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY` unset until `/opt/tellomi/secrets/db-backup-r2.env` exists. Without that file the script warns and uses the media key.

Once a month, restore onto a fresh Docker host and finish within two hours. That host is not Chengdu. The script and the record template are in [ops/RESTORE_DRILL.md](ops/RESTORE_DRILL.md). Do not copy the age identity onto this server.

## Memory limits

`docker-compose.limits.yaml` caps teamgram (3.5 GiB), mysql (1.5 GiB, InnoDB buffer pool 1 GiB), kafka (1.6 GiB, heap stays `-Xmx1G -Xms1G`), redis (512 MiB), etcd (256 MiB), coturn (256 MiB), and mediamtx (256 MiB). asr stays 1.5 GiB and 2 CPUs. Redis has no eviction policy, so at the cap the process is killed instead of dropping idgen or pts keys. Teamgram was 2.5 GiB; at that cap reclaim pinned a CPU and clients stayed on "connecting". Chengdu already runs 3584m.

Kafka on Chengdu is about 956 MiB RSS with that 1 GiB heap. The earlier 1.2 GiB cap and `-Xmx768m` were tighter than the process already runs, so the overlay was not applied to kafka. 1.6 GiB is the 1 GiB heap plus about 600 MiB for metaspace, threads, direct memory, and page cache. These caps are ceilings, not reservations. The host has 7.1G, and the caps can sum past that without every service sitting at its cap. Detail and the coturn entrypoint note are in [ops/COMPOSE_LIMITS.md](ops/COMPOSE_LIMITS.md).

`deploy-pull.sh` adds that file only when `TELLOMI_COMPOSE_LIMITS=1` is exported in the shell that runs the script and the file is in the checkout. A line in `.env` is not enough. Until then, production limits stay as they are.

```bash
export TELLOMI_COMPOSE_LIMITS=1
/opt/tellomi/deploy-pull.sh <sha7>
```

Copy `deploy-pull.sh` to `/opt/tellomi/deploy-pull.sh` after this change. The overlay itself comes from the checkout.

Render the same files the script uses. Unset, kafka has no `mem_limit` and no `KAFKA_HEAP_OPTS` (the image default heap is already 1G). Set to `1`, the overlay is included and kafka is 1.6g with `-Xmx1G -Xms1G`.

```bash
files=(-f docker-compose-env.yaml -f docker-compose.yaml)
docker compose "${files[@]}" config

export TELLOMI_COMPOSE_LIMITS=1
if [[ "${TELLOMI_COMPOSE_LIMITS}" == "1" ]]; then
  files+=(-f docker-compose.limits.yaml)
fi
docker compose "${files[@]}" config
bash scripts/check-compose-limits.sh
```

## Known follow-ups

- Remove the upstream RSA key from `gnetway.yaml` `RSAKey` once clients carrying the new public key are widespread.
- Chengdu does not pick up `deploy-pull.sh` or `scripts/ops/` from the checkout. Install them with the commands under Root disk before the next deploy.

## Datacenter addresses

`help.getConfig` `dc_options` stays the empty list in `teamgramd/bin/config.json` unless `TELLOMI_DC_OPTIONS` is set. Clients that already stored an address keep it after the server list goes back to empty. See [DC addresses](ops/DC_ADDRESSES.md).

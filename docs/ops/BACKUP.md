# Database backups

D-34, D-53, M7. MySQL, etcd, Redis, and the content-engine Postgres database are dumped every day, encrypted with age, and uploaded to a dedicated Cloudflare R2 bucket. This file has no secret values. The Postgres container defaults to `tellomi-feed-db`. If that container is missing or stopped, the run logs an alert and exits non-zero after the MySQL, etcd, and Redis uploads. `off`, `-`, or an empty `TELLOMI_POSTGRES_*` value is the only skip, and that skip is logged.

The script in git is [`scripts/ops/tellomi-db-backup.sh`](../../scripts/ops/tellomi-db-backup.sh). Chengdu does not pick it up from the checkout. Install it by hand, the same way as the disk scripts in [SECURITY-OPS.md](../SECURITY-OPS.md).

## What a run does

Cron runs the script at 03:15 as root.

| Data | How | Object prefix |
|---|---|---|
| MySQL database `teamgram` | `mysqldump` inside the `mysql` container (`--single-transaction`), gzip, age | `mysql/` |
| etcd | `etcdctl snapshot save` into the `/etcd-data` mount, then delete that file on the host, gzip, age | `etcd/` |
| Redis | `BGSAVE`, wait until `LASTSAVE` changes, then a read-only copy of `dump.rdb` and `appendonlydir`, tar, gzip, age | `redis/` |
| content-engine Postgres | `pg_dump -Fc` inside `tellomi-feed-db`, gzip, age. A missing container alerts and fails the run | `postgres/` |

Redis commands the script may send are `BGSAVE` and `LASTSAVE`. It does not send `FLUSHALL`, `FLUSHDB`, `FLUSH`, `CONFIG`, `DEBUG`, `SHUTDOWN`, or `SAVE`. `dump.rdb` and `appendonlydir` hold the idgen and pts counters (`message_box_ngen_*`, `pts_updates_ngen_*`). Those files are copied. Redis is not told to drop or rewrite keys.

Each ciphertext file gets a `.sha256` sidecar (`sha256sum -c` format). The run also writes `manifests/<stamp>.json` with the file names, sha256, sizes, and the git SHA of `TELLOMI_DEPLOY_DIR` (default `/opt/tellomi/teamgram-server`). The manifest is not encrypted. It has no keys.

Local directory: `/opt/tellomi/backups/daily/{mysql,etcd,redis,postgres,manifests}/`, mode `0700`, files mode `0600`. Files older than 90 days are deleted there. The script does not delete objects in R2. Remote expiry is the bucket lifecycle below.

Plaintext dumps stay in a `mktemp` directory and are removed when the run exits. The age identity is not on this host.

## Install on Chengdu

From a checkout of the commit you are deploying:

```bash
install -m 755 scripts/ops/tellomi-backup-common.sh /usr/local/bin/tellomi-backup-common.sh
install -m 755 scripts/ops/tellomi-db-backup.sh /usr/local/bin/tellomi-db-backup.sh
install -m 644 scripts/ops/tellomi-db-backup.cron /etc/cron.d/tellomi-db-backup
install -d -m 700 /opt/tellomi/backups/daily
```

`/usr/local/bin/tellomi-db-backup.sh` and `tellomi-backup-common.sh` must sit in the same directory. The script sources the common file next to itself.

Tools on the host: `docker`, `gzip`, `age`, `rclone` (Chengdu is 1.60.1), `sha256sum`, `tar`, GNU `find`, `flock`, `bash` 4.4 or newer. `age` and `rclone` belong on `PATH` (the cron file sets `/usr/local/bin`).

Check the copy, then a dry run, then one real run:

```bash
/usr/local/bin/tellomi-db-backup.sh --self-test
/usr/local/bin/tellomi-db-backup.sh --dry-run
/usr/local/bin/tellomi-db-backup.sh
```

`--self-test` does not need Docker, age, or R2. `--dry-run` prints the mysqldump, etcd snapshot, `BGSAVE`, `pg_dump -Fc`, the default container `tellomi-feed-db`, and rclone flags, and prints which local files are older than 90 days. It does not dump, encrypt, upload, or delete. A real run with a missing container still uploads MySQL, etcd, and Redis, then exits non-zero.

`git checkout` does not update `/usr/local/bin` or `/etc/cron.d`. Copy the three files again after they change.

## R2

Bucket: `tellomi-prod-db-backups`. One bucket, not the media buckets.

Create it in the Cloudflare dashboard (the S3 `CreateBucket` API returns `501 NotImplemented`, which is why rclone must not try).

Token for the daily job: **Object Read & Write**, scoped only to `tellomi-prod-db-backups`. D-34 asks for a write-only token so this job cannot read media. Cloudflare's object write preset cannot list or checksum, and the 04:00 check has to do both, so the daily token is Object Read & Write on this bucket alone. It is not an account admin token and it is not the media token. Do not grant access to any other bucket.

A separate **Object Read** token is for the monthly restore drill, stored with the age identity, not on Chengdu. The drill document covers that token.

Lifecycle (R2, not rclone):

| Rule | Prefix | Action |
|---|---|---|
| MySQL expiry | `mysql/` | Delete after 90 days |
| etcd expiry | `etcd/` | Delete after 90 days |
| Redis expiry | `redis/` | Delete after 90 days |
| Postgres expiry | `postgres/` | Delete after 90 days |
| Manifest expiry | `manifests/` | Delete after 90 days |
| Incomplete multipart | (whole bucket) | Abort after 1 day |

Bucket Lock: 90 days. The lock stops an overwrite or an early delete. Lifecycle deletes once the lock period for that object has passed. Both periods are 90 days from object creation; if a lifecycle pass races the lock by a few minutes, the next daily pass deletes the object. Do not add a scripted `rclone delete`. The previous host script used `--min-age 30d`. That is gone. Retention is 90 days (D-53).

### Manual step: `postgres/` lock and expiry

The backup script uploads `postgres/` and does not create R2 rules. Cloudflare has to be set by hand before those objects are protected. In the Cloudflare dashboard, bucket `tellomi-prod-db-backups`:

1. Add a lifecycle rule named `postgres expiry`: prefix `postgres/`, delete objects after 90 days. This is the same action as the `mysql/`, `etcd/`, `redis/`, and `manifests/` rows in the table above.
2. Add a Bucket Lock rule for prefix `postgres/`, retention 90 days. The lock stops an overwrite or an early delete. Lifecycle deletes the object once that lock period has passed.

Until both rules exist, a `postgres/` object can be overwritten or deleted early, and it will not expire on its own. The script cannot add these rules (R2 `CreateBucket` and lock APIs are not what this token is allowed to call). Do this in the dashboard. Do not add a scripted `rclone delete`.

Write `/opt/tellomi/secrets/db-backup-r2.env` mode `0600`, owner root. `KEY=value` lines only. The script does not execute the file.

```bash
install -d -m 700 /opt/tellomi/secrets
umask 077
cat >/opt/tellomi/secrets/db-backup-r2.env <<'EOF'
RCLONE_CONFIG_R2_TYPE=s3
RCLONE_CONFIG_R2_PROVIDER=Cloudflare
RCLONE_CONFIG_R2_ACCESS_KEY_ID=replace-with-the-backup-token-id
RCLONE_CONFIG_R2_SECRET_ACCESS_KEY=replace-with-the-backup-token-secret
RCLONE_CONFIG_R2_ENDPOINT=https://ACCOUNT_ID.r2.cloudflarestorage.com
RCLONE_CONFIG_R2_REGION=auto
RCLONE_CONFIG_R2_ACL=private
RCLONE_CONFIG_R2_NO_CHECK_BUCKET=true
EOF
chmod 600 /opt/tellomi/secrets/db-backup-r2.env
```

Replace the three placeholders. Do not commit the file. The endpoint is `https://<accountid>.r2.cloudflarestorage.com`.

Until that file exists, the script logs a warning and reads `TELLOMI_S3_ACCESS_KEY_ID` / `TELLOMI_S3_SECRET_ACCESS_KEY` / `TELLOMI_S3_ENDPOINT` from the `tellomi-teamgram-1` container. That is the media key. It keeps today's backups working. After the dedicated token is in place and one real run has uploaded with it, set `TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1` in the cron file (or the environment) so a missing dedicated file is a hard failure. Do not set that switch before the file exists.

## rclone 1.60.1 and R2

Chengdu's rclone is 1.60.1. Every `copyto` and `lsjson` gets these flags.

`--s3-no-check-bucket`

rclone's first S3 call otherwise ensures the bucket exists, and on this client that is `CreateBucket`. R2 does not implement `CreateBucket` and answers `501 NotImplemented`. An Object Read & Write token cannot create buckets either. The bucket must already exist.

`--s3-upload-cutoff 5G` and `--s3-acl private`

The 1.60.1 default cutoff is 200Mi. Larger files go through multipart upload, which this rclone has answered with `501` for headers R2 does not implement. `--s3-acl private` is required because R2 returns 501 for any canned ACL other than `private`. `5G` is the largest cutoff rclone allows and the S3 single-PUT limit, so a dump under 5GiB stays one PUT. A larger dump would use multipart; upgrade rclone rather than lowering the cutoff. Override with `TELLOMI_RCLONE_UPLOAD_CUTOFF`.

`--s3-no-head`

On Chengdu, a 2KB upload with rclone 1.60.1 and no extra flags logs `501 NotImplemented` and the object still lands. The same upload with only `--s3-no-head` succeeds and does not log that 501. rclone HEADs the object after PUT to compare the ETag. That HEAD is what R2 answers with 501. The `.sha256` sidecar is the integrity check, so the HEAD is not needed.

Do not pass `--s3-use-presigned-request`. On the same host a presigned PUT of a 2KB object is `400 Bad Request`, whether or not `--s3-no-head` is also set.

The env file also sets `RCLONE_CONFIG_R2_NO_CHECK_BUCKET=true` and `RCLONE_CONFIG_R2_ACL=private`. The command-line flags still win if a stale `rclone.conf` remote says otherwise.

## Age keys

Recipient (public, on the server, default in the script):

`age1wm306jrc25rl0lavlte3m8ctqn5l5p0lmjnhtgfeeh3nufvljs6q8ypr7t`

The private identity is offline, in the team vault (`成都服务器-运行时/`), not on Chengdu, not in git, and not in `/opt/tellomi/secrets`. The server can encrypt and can check the age header. It cannot decrypt. Decryption happens on the drill host, and the identity is removed from that host when the drill is over.

`TELLOMI_BACKUP_AGE_RECIPIENT` overrides the recipient if the key is rotated. Rotation means a new identity offline, the new public key on the server, and the old identity kept until every object it can decrypt has expired.

## Useful environment

| Variable | Default |
|---|---|
| `TELLOMI_BACKUP_DIR` | `/opt/tellomi/backups/daily` |
| `TELLOMI_BACKUP_RETAIN_DAYS` | `90` |
| `TELLOMI_BACKUP_R2_ENV` | `/opt/tellomi/secrets/db-backup-r2.env` |
| `TELLOMI_BACKUP_R2_REMOTE` | `r2:tellomi-prod-db-backups` |
| `TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY` | unset (media-key fallback). `1` fails closed |
| `TELLOMI_TEAMGRAM_CONTAINER` | `tellomi-teamgram-1` |
| `TELLOMI_REDIS_DATA` | `/opt/tellomi/teamgram-server/data/redis` |
| `TELLOMI_ETCD_DATA` | `/opt/tellomi/teamgram-server/data/etcd` (host path of `/etcd-data`) |
| `TELLOMI_RCLONE_UPLOAD_CUTOFF` | `5G` |
| `TELLOMI_BGSAVE_TIMEOUT` | `180` seconds |
| `TELLOMI_POSTGRES_CONTAINER` | `tellomi-feed-db`. `off`, `-`, or empty skips the dump and logs that skip |
| `TELLOMI_POSTGRES_DB` | `engine`. Empty skips the dump and logs that skip |
| `TELLOMI_POSTGRES_USER` | `engine`. Empty skips the dump and logs that skip |

Container names `mysql`, `etcd`, and `redis` match `container_name` in `docker-compose-env.yaml`. The content-engine container name is `tellomi-feed-db` in the Chengdu compose. Database and user stay `engine`.

The password is `POSTGRES_PASSWORD` inside the container. The script does not take it as an argument and does not log it.

`pg_dump -Fc` writes a custom-format archive, then gzip and age, same as the other segments. The object name is `postgres/<database>-<stamp>.dump.gz.age`. If `tellomi-feed-db` is missing or stopped, syslog gets an `ALERT` at `user.err`, MySQL, etcd, and Redis still upload, and the process exits non-zero. The manifest then has no `postgres` row. If the container is running and `pg_dump` fails, the whole run fails, the same way a failed `mysqldump` does. A host that does not run content-engine sets `TELLOMI_POSTGRES_CONTAINER=off` so the skip is intentional. The 04:00 check uses that same switch. With the default, a missing `postgres/` object fails the check even when the container is down.

## Verify a run

After the first real run:

```bash
ls -l /opt/tellomi/backups/daily/mysql /opt/tellomi/backups/daily/etcd /opt/tellomi/backups/daily/redis /opt/tellomi/backups/daily/postgres /opt/tellomi/backups/daily/manifests
sha256sum -c /opt/tellomi/backups/daily/mysql/teamgram-*.sql.gz.age.sha256
head -n 1 /opt/tellomi/backups/daily/mysql/teamgram-*.sql.gz.age
```

The first line of each ciphertext file is `age-encryption.org/v1`. That is a header check, not decryption.

```bash
rclone lsjson r2:tellomi-prod-db-backups/mysql/ --s3-no-check-bucket | head
```

The newest `mysql/`, `etcd/`, `redis/`, and `postgres/` objects should be from this run. There is no `rclone delete` in the script; confirm with `grep rclone /usr/local/bin/tellomi-db-backup.sh`. A missing `postgres/` object means the run alerted.

A monthly restore on a fresh Docker host is required. Do it off Chengdu. Steps, the read-only token, and the record template are in [RESTORE_DRILL.md](RESTORE_DRILL.md).

## 04:00 check

[`scripts/ops/tellomi-backup-verify.sh`](../../scripts/ops/tellomi-backup-verify.sh) lists the newest `.age` object under `mysql/`, `etcd/`, `redis/`, and `postgres/`. It does not decrypt. It does not look at Docker to decide whether `postgres/` can be skipped. A missing or stale `postgres/` object is an alert and a failed check, the same as `mysql/`. `TELLOMI_POSTGRES_CONTAINER=off` (or `-`, or empty) is the only skip, and the script logs it. For each prefix it checks:

- the object is younger than 26 hours (`TELLOMI_BACKUP_MAX_AGE_HOURS`);
- the `.sha256` sidecar matches the downloaded bytes (`sha256sum -c`);
- the first lines are an age header (`age-encryption.org/v1`, a `->` recipient, then `--- <MAC>`);
- the size is at least `TELLOMI_BACKUP_MIN_BYTES` (default 1024) and at most `TELLOMI_BACKUP_MAX_BYTES` (default 8 GiB).

The same rclone flags as the upload are used, including `--s3-no-check-bucket`. Credentials are the same env file, with the same media-key warning. A write-only token cannot list or download, which is why the daily token is Object Read & Write on this bucket.

At 04:00 a backup that finished at 03:15 the previous day is about 25 hours old, so one missed night still passes. The following morning it is about 49 hours old and the check fails. Set `TELLOMI_BACKUP_MAX_AGE_HOURS=6` in the cron file if a missed night should fail the same morning.

```bash
install -m 755 scripts/ops/tellomi-backup-verify.sh /usr/local/bin/tellomi-backup-verify.sh
install -m 644 scripts/ops/tellomi-backup-verify.cron /etc/cron.d/tellomi-backup-verify
```

`tellomi-backup-common.sh` must already be in `/usr/local/bin`. The script logs to syslog with tag `tellomi-backup-verify` and exits non-zero when a prefix fails.

```bash
/usr/local/bin/tellomi-backup-verify.sh --self-test
/usr/local/bin/tellomi-backup-verify.sh
```

`--self-test` uses a fake rclone. It does not talk to R2.

# Restore drill

D-34, D-53, M7. Once a month, restore the newest MySQL, etcd, and Redis backups onto a fresh Docker host and finish in under two hours. Do not run this on Chengdu. The script refuses the compose project `tellomi` and refuses to continue when `tellomi-teamgram-1` already exists.

The age identity and a read-only R2 token are used here and nowhere else. Take them off the drill host when the record is written. Chengdu keeps only the age public key.

## Before you start

A new VM with Docker, git, Go, `age`, `rclone` 1.60.1 or newer, `python3`, and `gzip`. Log in to the container registry so `docker compose pull` can fetch `teamgram` and `asr` by the 7-character tag. Clone this repo. The checkout does not need `docker-compose.tellomi.yaml`; that file is the Chengdu host override.

The read-only token is Cloudflare **Object Read** on `tellomi-prod-db-backups` only. Put it in a mode `0600` file, same keys as [BACKUP.md](BACKUP.md):

```bash
RCLONE_CONFIG_R2_TYPE=s3
RCLONE_CONFIG_R2_PROVIDER=Cloudflare
RCLONE_CONFIG_R2_ACCESS_KEY_ID=replace-with-the-read-token-id
RCLONE_CONFIG_R2_SECRET_ACCESS_KEY=replace-with-the-read-token-secret
RCLONE_CONFIG_R2_ENDPOINT=https://ACCOUNT_ID.r2.cloudflarestorage.com
RCLONE_CONFIG_R2_REGION=auto
RCLONE_CONFIG_R2_ACL=private
RCLONE_CONFIG_R2_NO_CHECK_BUCKET=true
```

`TELLOMI_IMAGE_TAG` is the 7-character image tag that was running when the backup was taken (the manifest's git SHA, shortened). The script checks that tag out and pulls it. It does not build.

## Run

```bash
export TELLOMI_IMAGE_TAG=<sha7>
export TELLOMI_DRILL_CONFIRM=restore-drill
export TELLOMI_BACKUP_AGE_IDENTITY=/path/to/offline-identity.txt
export TELLOMI_BACKUP_R2_ENV=/path/to/readonly.env
export TELLOMI_DRILL_OPERATOR='name'
export TELLOMI_DRILL_RECORD="$PWD/docs/ops/drills/$(date -u +%F).md"
bash scripts/ops/tellomi-restore-drill.sh
```

`--dry-run` prints the steps and writes a record without fetching or decrypting. `--self-test` checks the production refusals and does not need Docker.

| Step | What it does |
|---|---|
| install-tools | `age`, `rclone`, `docker`, `python3`, `go`, `gzip`, `tar`, `git` are on `PATH`. |
| pull-images | `git checkout` of `TELLOMI_IMAGE_TAG`, then `docker compose pull teamgram asr`. |
| fetch-backups | Newest `.age` under `mysql/`, `etcd/`, and `redis/`, plus the `.sha256` sidecar. The dedicated env file is required (`TELLOMI_BACKUP_REQUIRE_DEDICATED_KEY=1`). The media key is not a fallback. `postgres/` is the content-engine dump and is not part of this chat drill. |
| decrypt | `age -d -i` with the offline identity. Plaintext stays in a `mktemp` directory and is removed on exit. |
| restore-mysql | Start only `mysql` in project `tellomi-drill`, wait until it answers ping, import the dump. The root password is the container's `MYSQL_ROOT_PASSWORD`. |
| restore-redis | Extract `dump.rdb` and `appendonlydir` into `data/redis`, start `redis`, then `redis-cli --scan` for `pts_updates_ngen_*` and `message_box_ngen_*`. Both counts must be greater than zero. The script never sends `FLUSH`. |
| restore-etcd | `etcdctl snapshot restore` into `data/etcd` with the same member name as `docker-compose-env.yaml` (`node1`, `http://etcd:2380`), then start `etcd`. The image is `quay.io/coreos/etcd:v3.5.11` (`TELLOMI_DRILL_ETCD_IMAGE` overrides it). |
| compose-up | `docker compose -p tellomi-drill` for the rest of the stack. |
| handshake | `TestInitConnectionInvokeWithLayer` (MTProto handshake and `help.getConfig`). |
| login | `TestHelpGetConfig`. The client logs in with `auth.sendCode` and `auth.signIn` on the built-in `99966` test range. No SMS. |
| message | `TestPrivateChatReadOutbox`. One user sends a private message and the other reads it. |

The address is `TELLOMI_E2E_ADDR` (default `127.0.0.1:10443`).

Copy [drills/TEMPLATE.md](drills/TEMPLATE.md) if you are timing the steps by hand. The script writes the same table to `TELLOMI_DRILL_RECORD` and exits non-zero when the total is over 7200 seconds.

## After

Delete the identity and the read-only token from the drill host. Keep the record. Do not copy the restored data dir back to Chengdu. Tear the VM down, or `docker compose -p tellomi-drill down -v` if the disks are disposable. `container_name` in the compose files is `mysql`, `etcd`, `redis`, and `kafka`, so those names must not already belong to another project. A second drill on the same host has to remove the `tellomi-drill` containers first.

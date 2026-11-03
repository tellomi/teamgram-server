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

## Known follow-ups

- Remove the upstream RSA key from `gnetway.yaml` `RSAKey` once clients carrying the new public key are widespread.
- The `deploy-pull.sh` on the Chengdu host (`/opt/tellomi/deploy-pull.sh`) is older than the copy in this repo. Update the host file from this repo before relying on behavior that only the repo copy has.

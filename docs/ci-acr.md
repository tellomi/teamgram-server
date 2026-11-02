# CI images on Aliyun Container Registry

The Chengdu server must not compile images. Two local builds hung the machine.
GitHub Actions builds the image `docker-compose.yaml` used to build itself
(`teamgram`, from the root `Dockerfile`) and pushes it to ACR. Deploy is a pull.

The workflow runs on every push to `master` and from the Actions tab
(`workflow_dispatch`). A new push to the same ref cancels the in-progress
build, so an older run cannot finish later and move `:latest` backwards.

The image build downloads the ip2region v4 and v6 xdb from the official
`v3.18.0` tag, checks `deploy/ip2region/SHA256SUMS`, and copies them to
`/app/data/ip2region/`. A checksum mismatch fails the build. The DB-IP Lite
city mmdb is not in the image; `docker-compose.geoip.yaml` mounts it at
`/app/bin/GeoLite2-City.mmdb`. See `docs/ip2region.md`.

## Secrets

Set these as repository Actions secrets. The workflow logs in with
`docker/login-action` and pushes
`${ACR_REGISTRY}/${ACR_NAMESPACE}/teamgram`, tagged with the 7-character commit
SHA and `latest`.

| Secret | Purpose | When unset |
|---|---|---|
| `ACR_REGISTRY` | Registry host the workflow pushes to | `crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com` |
| `ACR_NAMESPACE` | ACR namespace | `tellomi` |
| `ACR_USERNAME` | Registry username | required |
| `ACR_PASSWORD` | Registry password | required |

The workflow pushes to the public endpoint
`crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com`. The application
server is not in the VPC, so Compose pulls that same host. The `-vpc` endpoint
is not reachable there.

`coturn` and `mediamtx` stay on their upstream images. This workflow does not
build them.

Buildx stores layer cache in GitHub Actions (`type=gha`). Provenance
attestations are off so ACR accepts the manifest.

## Deploy

On the server, from a checkout that contains `docker-compose.yaml`. Log in once
if the namespace is private:

```bash
docker login crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com
./deploy-pull.sh
```

On the production host the same script is `/opt/tellomi/deploy-pull.sh`.
A 7-character SHA argument pins the image and wins over `.env`:

```bash
/opt/tellomi/deploy-pull.sh <shortSHA>
```

`deploy-pull.sh` is the full deploy. It pulls `teamgram`, `coturn`, and
`mediamtx`, then recreates the stack from all three compose files. `teamgram`
depends on `kafka`, which is defined in `docker-compose-env.yaml`, so the app
file is not composed by itself. MinIO is profile `local-minio` and is not
started.

```bash
docker compose -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml pull teamgram coturn mediamtx
docker compose -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml up -d
```

`docker-compose.geoip.yaml` bind-mounts the DB-IP Lite city MMDB into
`teamgram`. Omit that `-f` and the next `up` drops the mount. The database
file is not in git. Before the first deploy with this script, set
`TELLOMI_GEOIP_MMDB_HOST` in `.env` to the mmdb already on the server. The
script exits if that file is missing. The container path defaults to
`/app/bin/GeoLite2-City.mmdb` (`TELLOMI_GEOIP_MMDB`). Leave it there: both
authsession and bff open `./GeoLite2-City.mmdb` from `/app/bin`.

The new image applies `teamgramd/deploy/sql/migrate-*.sql` before the processes
start. Pull this checkout as well so `docker-compose.yaml` passes
`MYSQL_ROOT_PASSWORD` into that step. The app DSN in `etc2` is unchanged.

`TELLOMI_IMAGE_REGISTRY` defaults to
`crpi-tge4xmtf4iip17mq.cn-chengdu.personal.cr.aliyuncs.com/tellomi`.
`TELLOMI_IMAGE_TAG` is the 7-character commit SHA. `deploy-pull.sh` sets it
from `git rev-parse --short=7 HEAD` when `.env` has it empty, `local`, or
`latest`. Do not deploy `latest`: that tag moves, and the running checkout
would not match the image. Do not pass `--build`. If `docker compose pull`
fails, fix the login or the registry. Do not build on this machine.

Dependency images (MySQL, Kafka, and the rest) are unchanged. MinIO is not
one of them. `minio` and `minio-mc` are Compose profile `local-minio`, so this
command does not pull or start them:

```bash
./deploy-pull.sh
```

Do not pass `--profile local-minio`. If an older copy of the script names
`minio` or `minio-mc`, remove those names. Nothing in the default project
`depends_on` MinIO. `teamgram` does not either. Keep
`-f docker-compose.geoip.yaml` on the same command as `up`.

## Link preview on Cloudflare R2

dfs already uses R2 through `TELLOMI_S3_*` ([storage.md](storage.md)).
linkpreview uses a separate client and will not follow those variables.
Set the keys below in the server `.env` before the next deploy. The endpoint
is the host only. `minio-go` adds `https` from `TELLOMI_MINIO_USE_SSL`.
Region `auto` is required. The three buckets are the same R2 bucket dfs
writes; object keys are `photos/<file_path>`, `documents/<file_path>`, and
`videos/<file_path>`. Use the same key pair as `TELLOMI_S3_ACCESS_KEY_ID` /
`TELLOMI_S3_SECRET_ACCESS_KEY`. Do not commit the secret.

```
TELLOMI_MINIO_ENDPOINT=<accountid>.r2.cloudflarestorage.com
TELLOMI_MINIO_REGION=auto
TELLOMI_MINIO_USE_SSL=true
TELLOMI_MINIO_ACCESS_KEY=<R2 access key id>
TELLOMI_MINIO_SECRET_KEY=<R2 secret access key>
TELLOMI_MINIO_BUCKET_PHOTOS=tellomi-attachments
TELLOMI_MINIO_BUCKET_DOCUMENTS=tellomi-attachments
TELLOMI_MINIO_BUCKET_VIDEOS=tellomi-attachments
```

`TELLOMI_MINIO_REGION` is read by the image from this change. Endpoint, SSL,
keys, and bucket names are read by older images too, but without `auto` those
images sign as `us-east-1` and R2 rejects the request. Pull the new image
before expecting `/linkpreview/v1/media` to read R2. Leave
`TELLOMI_S3_PUBLIC_MEDIA_BASE_URL` set when card URLs should be the public CDN.

## Rollback

Each push is tagged with the first 7 characters of the commit SHA (the short
SHA on the commit and on the Actions run). CI also moves `latest`, but deploy
does not pull `latest`.

To run an older build, set `TELLOMI_IMAGE_TAG` to that prefix and pull again.
For commit `9deb01fc…` the tag is `9deb01f`:

```bash
# .env
TELLOMI_IMAGE_TAG=9deb01f
```

```bash
./deploy-pull.sh
```

To run the checkout you have, remove `TELLOMI_IMAGE_TAG` or set it to
`local` and run `./deploy-pull.sh` again. The script replaces that with
`git rev-parse --short=7 HEAD`. Keep `-f docker-compose.geoip.yaml` on every
pull and up so the GeoIP mount stays.

## Local development

`build:` is still on the `teamgram` service. On a workstation:

```bash
docker compose up -d --build
```

That tags the local image with the same name Compose would pull. It does not
push.

# CI images on Aliyun Container Registry

The Chengdu server must not compile images. Two local builds hung the machine.
GitHub Actions builds the images `docker-compose.yaml` builds itself
(`teamgram` from the root `Dockerfile`, and `asr` from `services/asr`) and
pushes them to ACR. Deploy is a pull.

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
`${ACR_REGISTRY}/${ACR_NAMESPACE}/teamgram` and `.../asr`, each tagged with the
7-character commit SHA and `latest`.

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

On the production host the script is `/opt/tellomi/deploy-pull.sh`, a copy of
[`deploy-pull.sh`](../deploy-pull.sh). It takes the 7-character commit SHA,
which must already be published by the "Publish images" workflow on master:

```bash
/opt/tellomi/deploy-pull.sh <shortSHA>
```

The script lives outside the checkout. It works in `TELLOMI_DEPLOY_DIR`
(default `/opt/tellomi/teamgram-server`) and:

1. fetches and checks out `<shortSHA>` (detached), and pins
   `TELLOMI_IMAGE_TAG=<shortSHA>` in `.env`;
2. composes project `tellomi` from `docker-compose-env.yaml`,
   `docker-compose.yaml`, `docker-compose.geoip.yaml` and the host-only
   `docker-compose.tellomi.yaml` (loopback ports, coturn config, secrets from
   `.env`; not in git). It refuses to run without the override or the GeoIP
   MMDB;
3. pulls `teamgram`, plus `asr` when that commit defines it;
4. runs `mysql-migrate` and stops if it exits non-zero;
5. recreates `asr` (waiting up to 300s for its healthcheck), then `teamgram`.
   Kafka, MySQL, Redis, etcd, MinIO, coturn and mediamtx keep running.

The equivalent commands:

```bash
C="docker compose -p tellomi -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml -f docker-compose.tellomi.yaml"
$C pull asr teamgram
$C up --no-deps mysql-migrate
$C up -d --no-deps --wait --wait-timeout 300 asr
$C up -d --no-deps teamgram
```

`asr` is the SenseVoice-Small sidecar (`services/asr`). The int8 ONNX weights
are baked into the image at build time, so the Chengdu host does not download
them. The container is limited to 1.5GB and 2 CPUs. `deploy-pull.sh` already
pulls and recreates this image; it does not need a separate step. The model license is
`services/asr/NOTICE`.

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
`TELLOMI_IMAGE_TAG` is the 7-character commit SHA. `deploy-pull.sh` requires
it as the argument and writes it to `.env`. Do not deploy `latest`: that tag
moves, and the running checkout would not match the image. Do not pass `--build`. If `docker compose pull`
fails, fix the login or the registry. Do not build on this machine.

Dependency images (MySQL, Kafka, and the rest) are unchanged. MinIO is not
one of them. `minio` and `minio-mc` are Compose profile `local-minio`, and
the script only pulls and recreates `asr` and `teamgram`.

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

To roll back, deploy an older SHA whose image is still in ACR (and, for a
fast rollback, still on the host):

```bash
/opt/tellomi/deploy-pull.sh 63b10ef
```

The script checks out that commit, pins it in `.env`, and recreates `asr`
and `teamgram` from it. Migrations are forward-only; an older image runs
against the newer schema.

## Local development

`build:` is still on the `teamgram` service. On a workstation:

```bash
docker compose up -d --build
```

That tags the local image with the same name Compose would pull. It does not
push.

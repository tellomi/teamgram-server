# Staging on the same host

This file is the repo layout for a second Compose project. It is not a
record that the project is running. A 2026-11-07 operator read-only pass
found no staging containers, volumes, or `data-staging` directory on the
Chengdu host, and nothing listening on host `10444` or `11444`. That pass
is not authorization to create the stack. The bring-up steps, resource
caps, and the rule against copying production data are in
[STAGING_BRINGUP.md](ops/STAGING_BRINGUP.md).

Staging is a second Compose project. It does not reuse production's MySQL
data directory, published ports, or container names. The image is the one CI
pushed. The host does not build it.

Requires Docker Compose 2.24 or newer (`!override` in
`docker-compose.staging.yaml`).

## What is separate

| | Production | Staging |
|---|---|---|
| Project | directory basename, often `tellomi` | fixed `tellomi-staging` via `scripts/staging-compose.sh -p` |
| MTProto | host TCP 10443, all interfaces | host TCP **10444** on `127.0.0.1` only (container 10443) |
| WebSocket | host TCP 11443, all interfaces | host TCP **11444** on `127.0.0.1` only (container 11443) |
| MySQL data | `./data/mysql` | `./data-staging/mysql` |
| Redis / etcd / MinIO | `./data/...` | `./data-staging/...` |
| Kafka data | named volume `tellomi_kafka_data` | bind `./data-staging/kafka` (no named volume) |
| Network | `172.20.0.0/16` | `tellomi_staging_net` `172.21.0.0/16` |
| Container names | `mysql`, `teamgram`, … | `tellomi-staging-mysql`, `tellomi-staging-teamgram`, … |

The database *name* inside MySQL stays `teamgram`. The image DSN in
`teamgramd/etc2` is compiled in and uses hostname `mysql` on the compose
network. Isolation is the other data directory and the other container, not
a second schema name. Dependency ports are not published, so they do not
collide with production's 3306, 6379, 9092, or 9000.

asr, coturn, MediaMTX, MinIO, and the monitoring services are not in the
default staging project. asr is 1536m and 2 CPUs in the base file and would
start on a plain `up` unless this override sets `profiles: ["donotstart"]`.
The desktop chat set is etcd, kafka, mysql, mysql-migrate, redis, and
teamgram. The app still starts if Jaeger is absent.

## Deploy an image tag

Do not run `docker compose` yourself for this project. A checkout directory
named `tellomi`, or `COMPOSE_PROJECT_NAME=tellomi` left in the shell, selects
the production project and the volume `tellomi_kafka_data`. The top-level
`name:` does not win over that environment variable. `scripts/staging-compose.sh`
refuses a foreign `COMPOSE_PROJECT_NAME` and `COMPOSE_FILE`, then always
passes `-p tellomi-staging`.

This file does not authorize `up`. The resource, DC, and key checks are in
[STAGING_BRINGUP.md](ops/STAGING_BRINGUP.md).

```bash
export TELLOMI_IMAGE_TAG=180d555   # 7-character SHA from the Actions run
export STAGING_ENV_FILE=.env.staging

scripts/staging-compose.sh pull
scripts/staging-compose.sh up -d
```

Do not pass `--build`. Do not export `COMPOSE_PROJECT_NAME`.
`.env.staging` must set `TELLOMI_DC_OPTIONS=1:127.0.0.1:10444` for a desktop
SSH tunnel. `up` refuses an env file that omits it, and refuses an extra
RSA key. Leave `TELLOMI_MTPROTO_EXTRA_KEY_FILE` and
`TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT` empty.

`up` the first time creates `./data-staging` and loads SQL through the MySQL
entrypoint plus `mysql-migrate`. Later deploys only change
`TELLOMI_IMAGE_TAG` and pull again. The app container applies
`teamgramd/deploy/sql/migrate-*.sql` before the processes start.

To go back to the previous build, set `TELLOMI_IMAGE_TAG` to the older
7-character SHA and run the same pull and up.

## Smoke

Wait until `10444` accepts TCP, then run the post-deploy subset. The script
uses the test phones in `e2e/client/config.json` and does not restart the container.

```bash
TELLOMI_E2E_ADDR=127.0.0.1:10444 ./scripts/smoke-e2e.sh
```

Override a phone with `TELLOMI_E2E_PHONE_ALICE` and `TELLOMI_E2E_CODE_ALICE`
when the staging config does not use the default `99966` range.

## Stop

```bash
scripts/staging-compose.sh down
```

`down` through any other command line can select project `tellomi` when the
directory is named `tellomi`. That removes production containers.

`./data-staging` is the staging database. Removing it drops staging only.
Do not commit it.

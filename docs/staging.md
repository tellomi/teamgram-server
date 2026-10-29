# Staging on the same host

Staging is a second Compose project. It does not reuse production's MySQL
data directory, published ports, or container names. The image is the one CI
pushed. The host does not build it.

Requires Docker Compose 2.24 or newer (`!override` in
`docker-compose.staging.yaml`).

## What is separate

| | Production | Staging |
|---|---|---|
| Project | default (`docker compose`) | `COMPOSE_PROJECT_NAME=tellomi-staging` |
| MTProto | host port 10443 | host port 10444 |
| WebSocket | host port 11443 | host port 11444 |
| MySQL data | `./data/mysql` | `./data-staging/mysql` |
| Redis / etcd / MinIO | `./data/...` | `./data-staging/...` |
| Kafka volume | project volume `kafka_data` | project volume under `tellomi-staging` |
| Network | `172.20.0.0/16` | `tellomi_staging_net` `172.21.0.0/16` |
| Container names | `mysql`, `teamgram`, … | `tellomi-staging-mysql`, `tellomi-staging-teamgram`, … |

The database *name* inside MySQL stays `teamgram`. The image DSN in
`teamgramd/etc2` is compiled in and uses hostname `mysql` on the compose
network. Isolation is the other data directory and the other container, not
a second schema name. Dependency ports are not published, so they do not
collide with production's 3306, 6379, 9092, or 9000.

coturn, MediaMTX, and the monitoring services are left out of staging. They
bind host ports or the host network that production already uses. The app
still starts if Jaeger is absent.

## Deploy an image tag

On the server, from a checkout that contains the compose files. Log in to
the registry once if the namespace is private (`docs/ci-acr.md`).

```bash
export COMPOSE_PROJECT_NAME=tellomi-staging
export TELLOMI_IMAGE_TAG=180d555   # 7-character SHA from the Actions run

docker compose \
  -f docker-compose-env.yaml \
  -f docker-compose.yaml \
  -f docker-compose.staging.yaml \
  pull

docker compose \
  -f docker-compose-env.yaml \
  -f docker-compose.yaml \
  -f docker-compose.staging.yaml \
  up -d
```

Do not pass `--build`.

`up` the first time creates `./data-staging` and loads SQL through the MySQL
entrypoint plus `mysql-migrate`. Later deploys only change
`TELLOMI_IMAGE_TAG` and pull again. The app container applies
`teamgramd/deploy/sql/migrate-*.sql` before the processes start.

To go back to the previous build, set `TELLOMI_IMAGE_TAG` to the older
7-character SHA and run the same pull and up.

## Smoke

Wait until `10444` accepts TCP, then run the post-deploy subset. The script
uses the test phones in `e2e/config.json` and does not restart the container.

```bash
TELLOMI_E2E_ADDR=127.0.0.1:10444 ./scripts/smoke-e2e.sh
```

Override a phone with `TELLOMI_E2E_PHONE_ALICE` and `TELLOMI_E2E_CODE_ALICE`
when the staging config does not use the default `99966` range.

## Stop

```bash
COMPOSE_PROJECT_NAME=tellomi-staging docker compose \
  -f docker-compose-env.yaml \
  -f docker-compose.yaml \
  -f docker-compose.staging.yaml \
  down
```

`./data-staging` is the staging database. Removing it drops staging only.
Do not commit it.

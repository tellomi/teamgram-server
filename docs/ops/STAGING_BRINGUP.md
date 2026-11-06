# New isolated staging

This is a plan. It does not create the stack, change DNS, or change a
security group. Nothing in this file is a statement that staging is running.

On 2026-11-07 an operator read-only pass reported no staging containers,
volumes, or `data-staging` directory, and nothing listening on host `10444`
or `11444`. A separate 06:09:16 CST note of that host recorded 4 CPUs, 14 GiB
RAM with about 10 GiB available, disk `/dev/vda3` 69G with 26G free, and a
load average near 0.7. The mount list in that note is not a full `docker ps`
or `ss` inventory. It does show production data paths that a new project must
not reuse:

- `/opt/tellomi/teamgram-server/data/mysql`
- `/opt/tellomi/teamgram-server/data/redis`
- `/opt/tellomi/teamgram-server/data/etcd`
- `/opt/tellomi/teamgram-server/data/logs`
- Docker volume `tellomi_kafka_data`
- content-engine volumes `content-engine_pgdata` and `content-engine_media`

Do not mount `/opt/tellomi/secrets` into the staging project. Do not copy
production user rows, Redis dumps, Kafka data, or object storage.

## What the repo expects

`docker-compose.staging.yaml` is a second project, `tellomi-staging`:

| | Production file | Staging file |
| --- | --- | --- |
| Containers | names such as `mysql`, `teamgram` | `tellomi-staging-*` |
| Network | `172.20.0.0/16` | `172.21.0.0/16` |
| Data | `./data` | `./data-staging` |
| Host TCP | `10443` (container MTProto) | `127.0.0.1:10444` only |
| Host WebSocket | `11443` (container WebSocket) | `127.0.0.1:11444` only |

Container listeners stay `10443` TCP and `11443` WebSocket. Those numbers are
not the host ports. Public TCP `11443` is not staging MTProto.

Dependency ports are cleared (`ports: !override []`), so staging MySQL,
Redis, Kafka, and etcd are not published on the host. MinIO stays on the
`local-minio` profile and is left off. Do not point staging at production
R2 or at the content-engine.

## Capacity

The host is 4 CPUs and about 10 GiB RAM available, with 26G disk free at the
time of that note. Production already uses the machine. The staging file
caps the new processes so they cannot take the production ceilings in
`docker-compose.limits.yaml` (teamgram 3584m, MySQL 1536m, Kafka 1.6g):

| Service | Cap |
| --- | --- |
| teamgram | 1 CPU, 1024m |
| mysql | 512m, InnoDB buffer pool 256M |
| kafka | 768m, heap `-Xmx512m` |
| redis | 256m |
| etcd | 128m |

Those caps sum to about 2.7 GiB plus page cache. They are ceilings. Do not
include `docker-compose.limits.yaml` in the staging command.

Before `up`, on the host, record `nproc`, `free -h`, and `df -h /`. Abort if
available RAM is under 6 GiB or available disk is under 10 GiB. Abort if
`./data-staging` ever grows past 8 GiB. The app log driver stays
`max-size=50m`, `max-file=5`. gnetway file logs rotate daily and keep 7 days.
Do not bind staging logs onto `/opt/tellomi/teamgram-server/data/logs`.

## Side effects that stay off

Use a new env file, for example `.env.staging`, that is not the production
`.env` and is not committed.

- `TELLOMI_SMS_PROVIDER` unset or `fixed`. Never `aliyun`. Do not set
  `ALIBABA_CLOUD_ACCESS_KEY_ID` or `ALIBABA_CLOUD_ACCESS_KEY_SECRET`.
- `TELLOMI_EMAIL_PROVIDER=log`. Do not set `TELLOMI_RESEND_API_KEY`.
- Leave `TELLOMI_DISCOVER`, `TELLOMI_AI`, `TELLOMI_ANNOUNCE`, and the push
  credential paths unset. Do not mount the production APNs or MTProto key.
- The image default RSA key (fingerprint `12240908862933197005`) is the one
  current dev clients already have.
- Login tests use `99966XYYYY`. On DC 1 the code is `11111`. That range never
  calls the SMS API. Non-test numbers stay on the fixed code `12345` while
  the SMS provider is unset, and that path does not dial Aliyun.

`MYSQL_ROOT_PASSWORD` and `MYSQL_PASSWORD` in `.env.staging` must be new
values, not the production passwords. The database name inside the server
can stay `teamgram`. Isolation is the other container and `./data-staging`,
not a second schema on the production MySQL.

## Reachability without a public port

`10444` and `11444` bind to `127.0.0.1`. A desktop that already has SSH to
the host can forward them:

```bash
ssh -N -L 10444:127.0.0.1:10444 <ops-user>@<host>
```

The desktop then dials `127.0.0.1:10444`. That is MTProto TCP. WebSocket, if
needed, is local `11444`. This does not change DNS or a security group.

This path is feasible only if that SSH account allows local TCP forwarding.
The read-only pass did not check `AllowTcpForwarding`. Confirm that at
execution time. If it is off, stop. Do not publish the ports to `0.0.0.0`
in this step.

GitHub-hosted runners cannot use that tunnel. UI acceptance has no default
host. A self-hosted runner on the staging host can use `127.0.0.1` and
`10444` after `E2E_STAGING_CONFIRMED=tellomi-staging`. Do not point it at
`api.tellomi.cn` or port `10443`.

## Bring-up, later

Do not run this until a parent review says to. From a checkout that contains
these compose files, with `COMPOSE_PROJECT_NAME=tellomi-staging` and
`--env-file .env.staging`:

```bash
docker compose \
  --env-file .env.staging \
  -f docker-compose-env.yaml \
  -f docker-compose.yaml \
  -f docker-compose.staging.yaml \
  up -d
```

Do not pass `--build`. Do not pass `docker-compose.limits.yaml`. The first
`up` creates `./data-staging` and loads SQL through the MySQL entrypoint and
`mysql-migrate`. It does not read `./data/mysql`.

## Acceptance and rollback

On the host, after the containers exist:

1. `docker ps` names start with `tellomi-staging-`. No staging container
   mounts `/opt/tellomi/teamgram-server/data`.
2. `ss` shows `127.0.0.1:10444` and `127.0.0.1:11444` only, owned by
   `tellomi-staging-teamgram`. `10443`, `11443`, and `5222` stay on the
   production `teamgram` container.
3. TCP to `127.0.0.1:10444` succeeds on the host. Then
   `TELLOMI_E2E_ADDR=127.0.0.1:10444 ./scripts/smoke-e2e.sh` using the
   `99966` numbers in `e2e/client/config.json`.
4. A desktop through the SSH forward can complete login and one message on
   those test numbers. No production user is required.

Rollback is `docker compose ... down` for project `tellomi-staging` only.
Deleting `./data-staging` drops the new database and nothing under `./data`.
Do not `FLUSHALL` the production Redis. There is no production migration in
this plan. Image tags `c81f90ff` and `168c6f58` are not deployed by this
document.

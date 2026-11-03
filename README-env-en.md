# Dependency stack (`docker-compose-env.yaml`)

`docker-compose-env.yaml` is the dependency stack: MySQL, Redis, etcd, Kafka (KRaft, no ZooKeeper), and the optional monitoring services. The app is `docker-compose.yaml`. There is no `docker-compose-env2.yaml`.

MinIO is Compose profile `local-minio`. A default `up` does not start it. Chengdu media is Cloudflare R2, not this MinIO. See [docs/storage.md](docs/storage.md) and [docs/SECURITY-OPS.md](docs/SECURITY-OPS.md).

## Components

| Component | Description |
|-----------|-------------|
| kafka | Message queue (`apache/kafka:3.9.0`, KRaft, no ZooKeeper) |
| etcd | Configuration and discovery |
| redis | Cache |
| mysql | MySQL 8.0. Init SQL is already mounted from `teamgramd/deploy/sql`. |
| mysql-migrate | Applies `teamgramd/deploy/sql/migrate-*.sql` |
| minio / minio-mc | Profile `local-minio` only. Images `pgsty/silo` and `pgsty/mc`. |
| jaeger | Distributed tracing |
| prometheus | Metrics |
| node-exporter | Host metrics |
| grafana | Dashboards. Host port **3001** (container 3000). |
| elasticsearch | Log storage (named volume) |
| kibana | Log UI |
| filebeat | Log collection |
| go-stash | Kafka → Elasticsearch (`kevinwan/go-stash:1.1.1`) |

## Prerequisites

- Docker and Docker Compose (Compose V2: `docker compose`)
- For Elasticsearch, `vm.max_map_count` ≥ 262144 on Linux: `sysctl -w vm.max_map_count=262144`

## Environment variables

Copy `.env.example` to `.env`. The passwords in that file and in the table are **dev-only**. Production values were rotated on 2026-11-03 and are not in git. See [docs/SECURITY-OPS.md](docs/SECURITY-OPS.md).

| Variable | Dev default | Description |
|----------|-------------|-------------|
| MYSQL_ROOT_PASSWORD | root | MySQL root password |
| MYSQL_DATABASE | teamgram | Database name |
| MYSQL_USER | teamgram | Application DB user |
| MYSQL_PASSWORD | teamgram | Application DB password. Passed to the app as `TEAMGRAM_MYSQL_PASSWORD`. |
| MINIO_ROOT_USER | minio | Local MinIO user (profile `local-minio`) |
| MINIO_ROOT_PASSWORD | miniostorage | Local MinIO password |
| GRAFANA_ADMIN_USER | admin | Grafana admin user |
| GRAFANA_ADMIN_PASSWORD | admin | Grafana admin password |
| GRAFANA_ROOT_URL | http://localhost:3000 | Grafana root URL setting |

## Start and stop

```bash
docker compose -f docker-compose-env.yaml up -d
docker compose -f docker-compose-env.yaml --profile local-minio up -d   # workstation object storage
docker compose -f docker-compose-env.yaml ps
docker compose -f docker-compose-env.yaml down
```

`down` keeps bind-mounted data under `./data/` and the named volumes. Do not pass `--profile local-minio` on Chengdu.

## Ports

Published ports listen on all interfaces. They are not bound to `127.0.0.1` in this file. Chengdu adds `docker-compose.tellomi.yaml` (not in git) so infrastructure ports stay on loopback.

| Service | Host port | Notes |
|---------|-----------|--------|
| Jaeger UI | 16686 | |
| Prometheus | 9090 | Config `teamgramd/deploy/prometheus/server/prometheus.yml` |
| Grafana | 3001 | Container listens on 3000 |
| Kibana | 5601 | |
| MinIO API | 9000 | Profile `local-minio` only |
| MinIO console | 9001 | Profile `local-minio` only |
| MySQL | 3306 | |
| Redis | 6379 | |
| etcd | 2379 | |
| Kafka | 9092 and 9094 | Other containers use `kafka:9092`. Host clients use `127.0.0.1:9094`. |
| Elasticsearch | 9200 | |
| Node exporter | 9100 | |

## Configuration files

- Prometheus: `teamgramd/deploy/prometheus/server/prometheus.yml`
- Filebeat: `teamgramd/deploy/filebeat/conf/filebeat.yml`
- Go-stash: `teamgramd/deploy/go-stash/etc/config.yaml`

MySQL already mounts `teamgramd/deploy/sql` onto `/docker-entrypoint-initdb.d`. Do not add a `teamgramd/sql` volume. That directory is not the init path.

## Network

- Name: `teamgram_net`
- Driver: bridge
- Subnet: `172.20.0.0/16`

## Data

| Service | Where |
|---------|--------|
| kafka | named volume `kafka_data` |
| etcd | `./data/etcd` |
| redis | `./data/redis` |
| mysql | `./data/mysql` |
| minio | `./data/minio` (profile `local-minio`) |
| prometheus | `./data/prometheus/data` |
| grafana | `./data/grafana/data` |
| elasticsearch | named volume `elasticsearch_data` |

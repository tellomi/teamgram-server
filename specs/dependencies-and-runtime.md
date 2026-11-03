# Dependencies and runtime

This document covers core dependencies, recommended versions, and the Docker stack for Teamgram Server. The legacy component list is in [high-performance-components.md](high-performance-components.md) (redirect).

## Core dependencies (required)

| Component | Purpose | Recommended version |
|-----------|---------|---------------------|
| **MySQL** | Primary data store | 5.7 or 8.0 (Docker stack uses 8.0) |
| **Redis** | Cache, session, deduplication | 6.x / 7.x |
| **etcd** | Service discovery and config | v3.5.x |
| **Kafka** | Message and event pipeline | 3.x (KRaft, no Zookeeper) |
| **MinIO** | Object storage (documents, photos, videos) | Current stable |
| **FFmpeg** | Media transcoding (install on server) | Per install docs |

- Create database `teamgram` and run all init and migrate scripts under **`teamgramd/deploy/sql/`** in order.
- Local object storage is Compose profile `local-minio` (buckets `tellomi-attachments`, `tellomi-public-media`, `tellomi-backups`). Chengdu media is Cloudflare R2. See `docs/storage.md` and `docs/SECURITY-OPS.md`.

## Optional: monitoring and logging

The Docker stack in `docker-compose-env.yaml` can include:

| Component | Purpose |
|-----------|---------|
| **Jaeger** | Distributed tracing |
| **Prometheus** | Metrics |
| **Grafana** | Dashboards |
| **Node Exporter** | Host metrics |
| **Elasticsearch** | Log storage and search |
| **Kibana** | Log/data UI |
| **Filebeat** | Log collection (e.g. teamgram logs → Kafka) |
| **go-stash** | Log pipeline (Kafka → Elasticsearch) |

See the main README section “Logging, monitoring & tracing” and `teamgramd/deploy/` for config.

## Versions and Docker images (reference)

Aligned with `docker-compose-env.yaml` and [README-env-cn.md](../README-env-cn.md) / [README-env-en.md](../README-env-en.md):

- **Kafka**: apache/kafka:3.9.0 (KRaft, no ZooKeeper)
- **etcd**: quay.io/coreos/etcd:v3.5.11
- **Redis**: redis:7-alpine
- **MySQL**: mysql:8.0
- **MinIO** (profile `local-minio` only): docker.io/pgsty/silo and docker.io/pgsty/mc, tag `RELEASE.2026-10-26T00-00-00Z`
- **Jaeger**: jaegertracing/all-in-one:1.63.0
- **Prometheus**: prom/prometheus:v2.28.1
- **Grafana**: grafana/grafana:8.0.6 (host port 3001)
- **Node Exporter**: prom/node-exporter:v1.7.0
- **Elasticsearch / Kibana / Filebeat**: 8.12.2
- **go-stash**: kevinwan/go-stash:1.1.1

## Docker stack

- **Compose file**: **`docker-compose-env.yaml`**. There is no `docker-compose-env2.yaml`.
- **Usage**: Copy `.env.example` to `.env`, then `docker compose -f docker-compose-env.yaml up -d`. See README-env-cn.md / README-env-en.md. Dev passwords in that file are not production.
- **Network**: `teamgram_net`; data under project `data/` directory.

## Installation without Docker

If you install components yourself, see:

- [Manual installation (Linux)](../docs/install-manual-linux.md)
- [Manual installation (macOS)](../docs/install-manual-macos.md)

Then ensure MySQL, Redis, etcd, Kafka, MinIO, and FFmpeg are available and run `make` and `teamgramd/bin/runall2.sh`.

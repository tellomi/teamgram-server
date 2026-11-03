# Teamgram Server

[中文](README-zh.md) | **English**

---

Unofficial open-source [MTProto](https://core.telegram.org/mtproto) server implementation in Go. Compatible with Telegram clients; supports self-hosted deployment.

## Features

- **MTProto 2.0**
  - **Abridged**
  - **Intermediate**
  - **Padded intermediate**
  - **Full**
- **API Layer: 232** (vendored `proto/`, see [proto/README.md](proto/README.md))
- **Core features**
  - **private chat**
  - **basic group**
  - **contacts**
  - **web**

## Architecture

![Architecture](docs/image/architecture-001.png)

- [Architecture Docs](https://deepwiki.com/teamgram/teamgram-server) - `DeepWiki` teamgram/teamgram-server
- [Architecture (specs)](specs/architecture.md) — service topology, data flow, and ports  
- [Service topology and configuration](docs/service-topology.md) — ports, infrastructure dependencies, call graph, and Mermaid diagram ([中文](docs/service-topology-zh.md))

## Prerequisites

| Component | Purpose |
|-----------|---------|
| [MySQL](https://www.mysql.com/) 5.7+ / 8.0 | Primary data store |
| [Redis](https://redis.io/) | Cache, session, deduplication |
| [etcd](https://etcd.io/) | Service discovery & config |
| [Kafka](https://kafka.apache.org/) | Message & event pipeline |
| [MinIO](https://minio.io/) | Object storage |
| [FFmpeg](https://ffmpeg.org/) | Media transcoding (on server) |

Detailed versions and optional monitoring stack: [Dependencies and runtime (specs)](specs/dependencies-and-runtime.md).

---

## Manual installation

For running the server from source (Go build), follow the step-by-step guides:

- **[Manual installation (Linux)](docs/install-manual-linux.md)** — CentOS, Fedora, Ubuntu/Debian
- **[Manual installation (macOS)](docs/install-manual-macos.md)** — Intel and Apple Silicon

Requires Go 1.21+. You must install and configure dependencies (MySQL, Redis, etcd, Kafka, MinIO, FFmpeg), initialize the database and MinIO, then build and run.

---

## Docker installation

For running the full stack with Docker. **No manual data initialization:** the dependency stack initializes the database (via mounted SQL) and MinIO buckets (via `minio-mc`) on first start. Object storage is configured with `TELLOMI_S3_*` (MinIO locally, Cloudflare R2 in production); see [docs/storage.md](docs/storage.md). `TELLOMI_RTMP_URL` overrides the RTMP ingest URL returned by `phone.getGroupCallStreamRtmpUrl`.

Step-by-step for a layer-232 dev login (fixed SMS code `12345`): [Local deployment](docs/local-docker.md).

### 1. Clone

```bash
git clone https://github.com/tellomi/teamgram-server.git
cd teamgram-server
```

This clone is for a workstation. Chengdu does not deploy by cloning, and it does not fetch through a GitHub mirror. See [Security operations](docs/SECURITY-OPS.md).

### 2. Start dependency stack

This starts MySQL, Redis, etcd, Kafka (and optional monitoring). `--profile local-minio` also starts MinIO and creates its buckets. Production omits that profile and uses Cloudflare R2.

```bash
docker compose -f docker-compose-env.yaml --profile local-minio up -d
```

### 3. Start application

```bash
docker compose up -d --build
```

This builds the image on your machine and tags it `local` unless `TELLOMI_IMAGE_TAG` is set. The Chengdu server pulls a prebuilt image by the 7-character commit SHA instead ([CI and ACR](docs/ci-acr.md)). `deploy-pull.sh` does not pull `latest`.

---

## Logging, monitoring & tracing

| Area | Brief | Detailed docs |
|------|--------|----------------|
| **Log** | [Filebeat](https://www.elastic.co/beats/filebeat) → Kafka (`teamgram-log`) → [go-stash](https://github.com/kevwan/go-stash) → Elasticsearch → Kibana. Config: `teamgramd/deploy/filebeat/conf/filebeat.yml`, `teamgramd/deploy/go-stash/etc/`. | [Log collection](docs/log-collection.md) ([中文](docs/log-collection-zh.md)) |
| **Monitor** | [Prometheus](https://prometheus.io/) scrapes metrics; [Grafana](https://grafana.com/) for dashboards. Enable via `Prometheus` block in `teamgramd/etc2/*.yaml`. Config: `teamgramd/deploy/prometheus/server/prometheus.yml`. | [Service monitoring](docs/service-monitoring.md) ([中文](docs/service-monitoring-zh.md)) |
| **Tracing** | go-zero [Jaeger](https://www.jaegertracing.io/) / Zipkin support. Set `Telemetry` in `teamgramd/etc2/*.yaml`. Jaeger is included in `docker-compose-env.yaml`. | [Link tracking](docs/link-tracking.md) ([中文](docs/link-tracking-zh.md)) |

---

## Compatible clients

Tellomi Android is [tellomi/Telegram](https://github.com/tellomi/Telegram). Tellomi iOS is [tellomi/Telegram-iOS](https://github.com/tellomi/Telegram-iOS). The dev SMS provider accepts the fixed code `12345`. Production does not.

Notes under `clients/` describe the upstream Teamgram client forks. They are not the Tellomi apps.

---

## Documentation

- [Security operations](docs/SECURITY-OPS.md) — production RSA key, password locations, Chengdu deploy
- [Datacenter addresses](docs/ops/DC_ADDRESSES.md) — `help.getConfig` `dc_options` from `TELLOMI_DC_OPTIONS`
- [Project specs](specs/README.md) — Architecture, protocol, dependencies, contributing, security, roadmap
- [Service topology and configuration](docs/service-topology.md) — Ports, infrastructure, call graph ([中文](docs/service-topology-zh.md))
- [Client regression](docs/e2e.md) — MTProto suite, required CI check, post-deploy smoke
- [Staging](docs/staging.md) — second compose project on the same host
- [CONTRIBUTING](CONTRIBUTING.md) · [SECURITY](SECURITY.md) · [CHANGELOG](CHANGELOG.md)


---

## Community & feedback

- **Issues:** bugs and feature requests
- **Telegram:** [Teamgram group](https://t.me/+TjD5LZJ5XLRlCYLF)

---

## Operations

Production keys, where secrets are stored, and the Chengdu deploy path are in [Security operations](docs/SECURITY-OPS.md). Passwords committed in this repo (`teamgram` / `teamgram`, MySQL root `root`, MinIO user `minio`, Grafana user `admin`) are dev-only.

What this server implements is described in `docs/`. The old upstream Teamgram split between a community tree and an enterprise tree is not a map of this repository.

---

## License

[Apache License 2.0](LICENSE).

---

## Give a Star! ⭐

If this project helps you, consider giving it a star.

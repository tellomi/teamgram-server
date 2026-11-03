# Teamgram Server

**中文** | [English](README.md)

---

基于 Go 实现的开源 [MTProto](https://core.telegram.org/mtproto) 服务端，非官方实现。兼容 Telegram 客户端，支持私有化部署。

## 功能特性

- **MTProto 2.0**
  - **Abridged**
  - **Intermediate**
  - **Padded intermediate**
  - **Full**
- **API Layer: 232**（仓库内 `proto/`，见 [proto/README.md](proto/README.md)）
- **Core features**
  - **private chat**
  - **basic group**
  - **contacts**
  - **web**

## 架构

![架构图](docs/image/architecture-001.png)

- [Architecture Docs](https://deepwiki.com/teamgram/teamgram-server) - `DeepWiki` teamgram/teamgram-server
- [架构说明（specs）](specs/architecture.md) — 服务拓扑、数据流与端口  
- [服务拓扑与配置](docs/service-topology-zh.md) — 各服务端口、基础设施依赖、调用关系与 Mermaid 拓扑图（[English](docs/service-topology.md) 为主文档）

## 前置依赖

| 组件 | 用途 |
|------|------|
| [MySQL](https://www.mysql.com/) 5.7+ / 8.0 | 主数据存储 |
| [Redis](https://redis.io/) | 缓存、会话、去重 |
| [etcd](https://etcd.io/) | 服务发现与配置 |
| [Kafka](https://kafka.apache.org/) | 消息与事件管道 |
| [MinIO](https://minio.io/) | 对象存储 |
| [FFmpeg](https://ffmpeg.org/) | 媒体转码（需在服务端安装） |

版本建议与可选监控栈详见 [依赖与运行环境（specs）](specs/dependencies-and-runtime.md)。

**无 Docker 时的安装文档：**

- [手动安装（Linux）](docs/install-manual-linux-zh.md)
- [手动安装（macOS）](docs/install-manual-macos-zh.md)

**Docker 部署：** [本地 Docker](docs/local-docker.md)。成都生产见 [CI 与 ACR](docs/ci-acr.md) 与 [安全运维](docs/SECURITY-OPS.md)。

**一键环境（Docker）：** 使用 [docker-compose-env.yaml](docker-compose-env.yaml)，详见 [README-env-cn.md](README-env-cn.md) / [README-env-en.md](README-env-en.md)。

---

## 手动安装

从源码构建并运行服务时，请按以下文档逐步操作：

- **[手动安装（Linux）](docs/install-manual-linux-zh.md)** — CentOS、Fedora、Ubuntu/Debian
- **[手动安装（macOS）](docs/install-manual-macos-zh.md)** — Intel 与 Apple Silicon

需要 Go 1.21+。需自行安装并配置依赖（MySQL、Redis、etcd、Kafka、MinIO、FFmpeg），初始化数据库与 MinIO，再编译并运行。

---

## Docker 安装

使用 Docker 一键运行完整栈。**无需手动初始化数据**：依赖栈首次启动时会自动初始化数据库（挂载 SQL）和 MinIO 桶（通过 `minio-mc`）。对象存储由 `TELLOMI_S3_*` 配置（本地 MinIO，生产可用 Cloudflare R2），见 [docs/storage.md](docs/storage.md)。`TELLOMI_RTMP_URL` 覆盖 `phone.getGroupCallStreamRtmpUrl` 返回的 RTMP 推流地址。

### 1. 克隆仓库

```bash
git clone https://github.com/tellomi/teamgram-server.git
cd teamgram-server
```

这次克隆只用于本机。成都不靠克隆部署，也不从 GitHub 镜像拉取。见 [安全运维](docs/SECURITY-OPS.md)。

### 2. 启动依赖栈

将启动 MySQL、Redis、etcd、Kafka 及可选监控组件。加上 `--profile local-minio` 才会启动 MinIO 并创建桶。生产环境不要加这个 profile，对象存储用 Cloudflare R2。

```bash
docker compose -f docker-compose-env.yaml --profile local-minio up -d
```

### 3. 启动应用

```bash
docker compose up -d --build
```

该命令在本机构建镜像。成都服务器改为拉取已构建镜像，见 [CI 与 ACR](docs/ci-acr.md)。

---

## 日志、监控与链路追踪

| 模块 | 简述 | 详细文档 |
|------|------|----------|
| **日志** | Filebeat → Kafka（`teamgram-log`）→ go-stash → Elasticsearch → Kibana。配置：`teamgramd/deploy/filebeat/conf/filebeat.yml`、`teamgramd/deploy/go-stash/etc/`。 | [日志收集](docs/log-collection-zh.md)（[English](docs/log-collection.md)） |
| **监控** | Prometheus 拉取指标，Grafana 展示。在 `teamgramd/etc2/*.yaml` 中配置 `Prometheus`。配置：`teamgramd/deploy/prometheus/server/prometheus.yml`。 | [服务监控](docs/service-monitoring-zh.md)（[English](docs/service-monitoring.md)） |
| **链路追踪** | go-zero 支持 Jaeger / Zipkin，在 `teamgramd/etc2/*.yaml` 中配置 `Telemetry`。`docker-compose-env.yaml` 已包含 Jaeger。 | [链路追踪](docs/link-tracking-zh.md)（[English](docs/link-tracking.md)） |

---

## 兼容客户端

Tellomi 安卓是 [tellomi/Telegram](https://github.com/tellomi/Telegram)，iOS 是 [tellomi/Telegram-iOS](https://github.com/tellomi/Telegram-iOS)。开发环境短信固定码是 `12345`。生产环境不是。

`clients/` 下的说明是上游 Teamgram 客户端分支，不是 Tellomi 应用。

---

## 文档

- [安全运维](docs/SECURITY-OPS.md) — 生产 RSA 密钥、密码存放位置、成都部署
- [项目规范与设计文档（specs）](specs/README.md) — 架构、协议、依赖、贡献、安全、路线图
- [服务拓扑与配置](docs/service-topology-zh.md) — 端口、基础设施、调用关系（[English](docs/service-topology.md)）
- [CONTRIBUTING](CONTRIBUTING.md) · [SECURITY](SECURITY.md) · [CHANGELOG](CHANGELOG.md)

---

## 社区与反馈

- **Issues：** 缺陷与功能建议
- **Telegram：** [Teamgram 群组](https://t.me/+TjD5LZJ5XLRlCYLF)

---

## 运维

生产密钥、秘密存放位置和成都部署见 [安全运维](docs/SECURITY-OPS.md)。仓库里的密码（`teamgram` / `teamgram`、MySQL root `root`、MinIO 用户 `minio`、Grafana 用户 `admin`）只用于开发。

本服务实现了什么以 `docs/` 为准。上游 Teamgram 的社区版/企业版划分不能当作本仓库的功能边界。

---

## 许可证

[Apache License 2.0](LICENSE)。

---

## Star ⭐

若本项目对你有帮助，欢迎 Star。

# 依赖栈（`docker-compose-env.yaml`）

`docker-compose-env.yaml` 是依赖栈：MySQL、Redis、etcd、Kafka（KRaft，无 ZooKeeper），以及可选的监控组件。应用在 `docker-compose.yaml`。仓库里没有 `docker-compose-env2.yaml`。

MinIO 属于 Compose profile `local-minio`。默认 `up` 不会启动它。成都的媒体在 Cloudflare R2，不在这台 MinIO 上。见 [docs/storage.md](docs/storage.md) 与 [docs/SECURITY-OPS.md](docs/SECURITY-OPS.md)。

## 组件

| 组件 | 说明 |
|------|------|
| kafka | 消息队列（`apache/kafka:3.9.0`，KRaft，无 ZooKeeper） |
| etcd | 配置与发现 |
| redis | 缓存 |
| mysql | MySQL 8.0。初始化 SQL 已挂载 `teamgramd/deploy/sql`。 |
| mysql-migrate | 执行 `teamgramd/deploy/sql/migrate-*.sql` |
| minio / minio-mc | 仅 profile `local-minio`。镜像为 `pgsty/silo` 与 `pgsty/mc`。 |
| jaeger | 分布式追踪 |
| prometheus | 指标 |
| node-exporter | 主机指标 |
| grafana | 仪表盘。宿主机端口 **3001**（容器内 3000）。 |
| elasticsearch | 日志存储（命名卷） |
| kibana | 日志界面 |
| filebeat | 日志采集 |
| go-stash | Kafka → Elasticsearch（`kevinwan/go-stash:1.1.1`） |

## 前置要求

- Docker 与 Docker Compose（Compose V2：`docker compose`）
- 运行 Elasticsearch 时，Linux 上 `vm.max_map_count` ≥ 262144：`sysctl -w vm.max_map_count=262144`

## 环境变量

把 `.env.example` 复制为 `.env`。文件里和下面表里的密码只用于本机开发。生产密码已在 2026-11-03 轮换，不在仓库里。见 [docs/SECURITY-OPS.md](docs/SECURITY-OPS.md)。

| 变量 | 开发默认值 | 说明 |
|------|------------|------|
| MYSQL_ROOT_PASSWORD | root | MySQL root 密码 |
| MYSQL_DATABASE | teamgram | 数据库名 |
| MYSQL_USER | teamgram | 应用数据库用户 |
| MYSQL_PASSWORD | teamgram | 应用数据库密码。会作为 `TEAMGRAM_MYSQL_PASSWORD` 传给应用容器。 |
| MINIO_ROOT_USER | minio | 本机 MinIO 用户（profile `local-minio`） |
| MINIO_ROOT_PASSWORD | miniostorage | 本机 MinIO 密码 |
| GRAFANA_ADMIN_USER | admin | Grafana 管理员 |
| GRAFANA_ADMIN_PASSWORD | admin | Grafana 管理员密码 |
| GRAFANA_ROOT_URL | http://localhost:3000 | Grafana 根 URL 设置 |

## 启动与停止

```bash
docker compose -f docker-compose-env.yaml up -d
docker compose -f docker-compose-env.yaml --profile local-minio up -d   # 本机对象存储
docker compose -f docker-compose-env.yaml ps
docker compose -f docker-compose-env.yaml down
```

`down` 会保留 `./data/` 下的绑定挂载和命名卷。不要在成都加上 `--profile local-minio`。

## 端口

这个文件把端口发布到所有网卡，没有绑在 `127.0.0.1`。成都用仓库外的 `docker-compose.tellomi.yaml` 把基础设施端口限制在回环地址。

| 服务 | 宿主机端口 | 说明 |
|------|------------|------|
| Jaeger UI | 16686 | |
| Prometheus | 9090 | 配置 `teamgramd/deploy/prometheus/server/prometheus.yml` |
| Grafana | 3001 | 容器内监听 3000 |
| Kibana | 5601 | |
| MinIO API | 9000 | 仅 profile `local-minio` |
| MinIO 控制台 | 9001 | 仅 profile `local-minio` |
| MySQL | 3306 | |
| Redis | 6379 | |
| etcd | 2379 | |
| Kafka | 9092 与 9094 | 容器之间用 `kafka:9092`。宿主机客户端用 `127.0.0.1:9094`。 |
| Elasticsearch | 9200 | |
| Node exporter | 9100 | |

## 配置文件

- Prometheus：`teamgramd/deploy/prometheus/server/prometheus.yml`
- Filebeat：`teamgramd/deploy/filebeat/conf/filebeat.yml`
- Go-stash：`teamgramd/deploy/go-stash/etc/config.yaml`

MySQL 已经把 `teamgramd/deploy/sql` 挂到 `/docker-entrypoint-initdb.d`。不要再挂 `teamgramd/sql`，那不是初始化目录。

## 网络

- 网络名：`teamgram_net`
- 驱动：bridge
- 子网：`172.20.0.0/16`

## 数据

| 服务 | 位置 |
|------|------|
| kafka | 命名卷 `kafka_data` |
| etcd | `./data/etcd` |
| redis | `./data/redis` |
| mysql | `./data/mysql` |
| minio | `./data/minio`（profile `local-minio`） |
| prometheus | `./data/prometheus/data` |
| grafana | `./data/grafana/data` |
| elasticsearch | 命名卷 `elasticsearch_data` |

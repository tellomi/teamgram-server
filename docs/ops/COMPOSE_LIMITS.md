# Compose memory limits

D-34, M7. `docker-compose.limits.yaml` is an opt-in overlay. `deploy-pull.sh` appends it only when `TELLOMI_COMPOSE_LIMITS=1` is exported in the shell that runs the script and the file is in the checkout. A line in `.env` is not read. Unset leaves the running limits alone. The short version is in [SECURITY-OPS.md](../SECURITY-OPS.md#memory-limits).

| Service | Limit |
|---|---|
| teamgram | 3584m (3.5 GiB) |
| mysql | 1536m, InnoDB buffer pool 1 GiB |
| kafka | 1.6g, heap `-Xmx1G -Xms1G` |
| redis | 512m, no eviction |
| etcd | 256m |
| coturn | 256m |
| mediamtx | 256m |
| asr | 1536m and 2 CPUs, same as `docker-compose.yaml` |

## Teamgram

The cap was 2560m. On Chengdu the process sat on that cgroup: reclaim used a full CPU and clients stayed on "connecting". It is now 3584m (3.5 GiB), the limit already set on the host.

## Kafka

Chengdu's broker is about 956 MiB RSS with a 1 GiB Java heap. `apache/kafka:3.9.0` uses `-Xmx1G -Xms1G` when `KAFKA_HEAP_OPTS` is unset (`kafka-server-start.sh`). The first overlay set a 1.2g cgroup and lowered the heap to 768m. That is tighter than the process already runs: a full 1 GiB heap plus metaspace, thread stacks, and page cache does not fit in 1.2g, and 768m is a smaller heap than production. The overlay was not applied to kafka.

The cap is now 1.6g and the heap stays 1 GiB. 1.6 GiB is the heap plus about 600 MiB for that non-heap use, which covers the measured 956 MiB with room for the heap to reach 1 GiB. Kafka's generic container hint is heap plus 1 GiB (2g here). On a 7.1G host, 1.6g is the cap that still fits the measured broker. A later `up -d kafka` then stays under the cgroup instead of being OOM-killed.

`mem_limit` is a ceiling, not a reservation. The host has 7.1G. The caps above can sum past that (about 9.3 GiB). They do not all sit at the cap at once. Redis still has no eviction policy, so at 512m the process is killed instead of dropping idgen or pts keys. D-69's recount, including feed Postgres which is not in this file, is in [chat-standby.md](../design/chat-standby.md#memory-budget-d-69). The memory numbers in the table are unchanged.

## Host budget

The 7.1G figure is the Chengdu host size already written in this file, in [SECURITY-OPS.md](../SECURITY-OPS.md#memory-limits), and as "about 7.1 GiB" on an 8 GB host in the D-69 section. Nothing here SSHs to the machine or re-measures RAM. About 1 GiB of that 7.1G stays outside the container caps for the OS and the 03:15 backup (`mysqldump`, the etcd snapshot, the Redis file copy, `pg_dump` of `tellomi-feed-db`, gzip, and age).

`memswap_limit` is set to the same value as `mem_limit`. With only `mem_limit`, Docker lets the container swap another copy of that cap. On this host that swap comes out of the same 7.1G the OS and the backup need. `deploy.resources.limits.memory` stays equal to `mem_limit`.

| Service | Cap | Why |
|---|---|---|
| teamgram | 3584m | Largest process. 2560m pinned a CPU in reclaim and clients stayed on "connecting". Chengdu already runs 3584m. Not lowered. |
| mysql | 1536m | Next. The 1 GiB InnoDB buffer pool has to fit inside the cgroup. |
| kafka | 1.6g | About 956 MiB RSS and a 1 GiB heap. 1.2g was too small. |
| redis | 512m | Small. No eviction, so a kill is preferred to dropping idgen or pts keys. |
| etcd | 256m | Discovery data is small. |
| coturn, mediamtx | 256m each | Sidecars. |
| asr | 1536m | Already set in `docker-compose.yaml`. The overlay repeats it. |

## content-engine

content-engine is not a service in `docker-compose.limits.yaml` or in `docker-compose.yaml`. `https://github.com/tellomi/content-engine` was not readable from this checkout (API 404). This repo only names the Chengdu database container `tellomi-feed-db` (database and user `engine`). Proposed cap for that container, applied in the content-engine compose and not in this overlay: **512m**. Same order of magnitude as Redis, under MySQL's 1536m, and it does not lower teamgram. `pg_dump -Fc` runs inside that container, so the cap has to cover the server and the dump.

The engine HTTP process is a different container. Its compose service name is not in this repo, so this file does not invent one and does not set a cap for it. `ENGINE_COLIKE` is not set here and stays off. Do not add `tellomi-feed-db` to this overlay. D-69 still says the feed database does not fit on top of the table until something else leaves the host. 512m is the number for that other compose.

## Apply

This overlay does not deploy, and it does not add an image-prune cron. `deploy-pull.sh` recreates `asr` and `teamgram` only, and only when `TELLOMI_COMPOSE_LIMITS=1` is exported in that process. mysql, redis, etcd, kafka, coturn, and mediamtx keep the cgroup they already have until a later `up -d` of those names. Image cleanup stays the existing disk script in [SECURITY-OPS.md](../SECURITY-OPS.md#root-disk).

## Verify

`docker compose config` does not read `TELLOMI_COMPOSE_LIMITS`. The variable only tells `deploy-pull.sh` to add `-f docker-compose.limits.yaml`. Render both shapes:

```bash
files=(-f docker-compose-env.yaml -f docker-compose.yaml)

# TELLOMI_COMPOSE_LIMITS unset. Kafka has no mem_limit and no KAFKA_HEAP_OPTS.
docker compose "${files[@]}" config

# TELLOMI_COMPOSE_LIMITS=1. Kafka mem_limit is 1.6g and KAFKA_HEAP_OPTS is -Xmx1G -Xms1G.
export TELLOMI_COMPOSE_LIMITS=1
if [[ "${TELLOMI_COMPOSE_LIMITS}" == "1" ]]; then
  files+=(-f docker-compose.limits.yaml)
fi
docker compose "${files[@]}" config

bash scripts/check-compose-limits.sh
```

Compose renders `1.6g` as 1717986918 bytes (`1.6 * 1024^3`). `deploy.resources.limits.memory` and `memswap_limit` are the same number.

## coturn entrypoint

The base file bind-mounts `teamgramd/deploy/coturn/entrypoint.sh` and runs it when `/opt/coturn/entrypoint.sh` is present. A host override that sets `volumes: !override` drops that mount. The entrypoint used to be only that path, so coturn crash-looped. Chengdu set `entrypoint: !reset null` in `docker-compose.tellomi.yaml`.

The entrypoint is now conditional. Missing script means the image `docker-entrypoint.sh` and the image command, which is what `entrypoint: !reset null` already starts. Mount `/etc/coturn/turnserver.conf` in the override when the generator is not mounted. The image reads that path. `entrypoint: !reset null` can stay; it is no longer required. To keep the `TELLOMI_TURN_*` generator, repeat the script mount in the override. See [calls.md](../calls.md).

```bash
docker compose -f docker-compose-env.yaml -f docker-compose.yaml config
```

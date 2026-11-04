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

`mem_limit` is a ceiling, not a reservation. The host has 7.1G. The caps above can sum past that (about 9.3 GiB). They do not all sit at the cap at once. Redis still has no eviction policy, so at 512m the process is killed instead of dropping idgen or pts keys. D-69's recount, including feed Postgres which is not in this file, is in [chat-standby.md](../design/chat-standby.md#memory-budget-d-69). The numbers in this file are unchanged.

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

Compose renders `1.6g` as 1717986918 bytes (`1.6 * 1024^3`). `deploy.resources.limits.memory` is the same number.

## coturn entrypoint

The base file bind-mounts `teamgramd/deploy/coturn/entrypoint.sh` and runs it when `/opt/coturn/entrypoint.sh` is present. A host override that sets `volumes: !override` drops that mount. The entrypoint used to be only that path, so coturn crash-looped. Chengdu set `entrypoint: !reset null` in `docker-compose.tellomi.yaml`.

The entrypoint is now conditional. Missing script means the image `docker-entrypoint.sh` and the image command, which is what `entrypoint: !reset null` already starts. Mount `/etc/coturn/turnserver.conf` in the override when the generator is not mounted. The image reads that path. `entrypoint: !reset null` can stay; it is no longer required. To keep the `TELLOMI_TURN_*` generator, repeat the script mount in the override. See [calls.md](../calls.md).

```bash
docker compose -f docker-compose-env.yaml -f docker-compose.yaml config
```

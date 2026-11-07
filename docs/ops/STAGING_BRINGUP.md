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

The 06:09:16 CST note is a point in time, not a live measurement: 4 CPUs,
about 10 GiB RAM available, 26G disk free, shared with production. Re-read
`nproc`, `free -h`, and `df -h /` on the host before any later `up`. Abort
if available RAM is under 6 GiB or available disk is under 10 GiB.

Desktop chat does not start asr, coturn, MediaMTX, MinIO, or monitoring.
The base file gives asr 1536m and 2 CPUs and no profile, so a raw `up` of
the combined files would have added that on top of the earlier ~2.7 GiB
figure. The staging override sets `profiles: ["donotstart"]` for asr.
`scripts/staging-compose.sh check` fails if asr is in the rendered service
set.

Default services and ceilings (`docker-compose.limits.yaml` is not used;
that file's production ceilings are teamgram 3584m, MySQL 1536m, Kafka 1.6g):

| Service | CPU | Memory | Disk | Logs |
| --- | --- | --- | --- | --- |
| teamgram | 1 | 1024m | `./data-staging/logs` | json-file 50m x 5, gnetway KeepDays 7 |
| mysql | 0.5 | 512m, InnoDB pool 256M | `./data-staging/mysql` | json-file 20m x 3 |
| kafka | 0.5 | 768m, heap `-Xmx512m` | `./data-staging/kafka` | json-file 50m x 3 |
| redis | 0.25 | 256m | `./data-staging/redis` | json-file 20m x 3 |
| etcd | 0.25 | 128m | `./data-staging/etcd` | json-file 20m x 3 |
| mysql-migrate | none | 256m, then exit | none of its own | one-shot |

Those memory ceilings sum to 2944 MiB. CPU sum is 2.5 of the host's 4.
Kafka is a bind under `./data-staging`, not a Docker named volume. The
8 GiB cap is `du -sb ./data-staging` and therefore includes Kafka. Abort
before `up`, and stop after, if that directory is over 8 GiB. A named
volume such as `tellomi_kafka_data` is outside that directory and is a
failed check. Do not bind staging logs onto
`/opt/tellomi/teamgram-server/data/logs`.

## Side effects that stay off

Use a new env file, for example `.env.staging`, that is not the production
`.env` and is not committed.

- `TELLOMI_SMS_PROVIDER` unset or `fixed`. Never `aliyun`. Do not set
  `ALIBABA_CLOUD_ACCESS_KEY_ID` or `ALIBABA_CLOUD_ACCESS_KEY_SECRET`.
- `TELLOMI_EMAIL_PROVIDER=log`. Do not set `TELLOMI_RESEND_API_KEY`.
- Leave `TELLOMI_DISCOVER`, `TELLOMI_AI`, `TELLOMI_ANNOUNCE`, and the push
  credential paths unset. Do not mount the production APNs or MTProto key.
- Leave `TELLOMI_MTPROTO_EXTRA_KEY_FILE` and
  `TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT` empty. Do not copy the production
  private key and do not mount `/opt/tellomi/secrets`. See the key section
  below. This PR does not change any client pin.
- For a desktop that dials the SSH forward, set
  `TELLOMI_DC_OPTIONS=1:127.0.0.1:10444` and `TELLOMI_DC_TXT_DOMAIN=-`.
  That is the only value `scripts/staging-compose.sh up` accepts. The check
  parses the list. It refuses a production address before or after the
  tunnel, a duplicate, an empty item, IPv6, whitespace, and any other DC.
  Unset or empty leaves `help.getConfig` `dc_options` empty
  (`TestHelpGetConfigDCOptionsUnsetMatchesFile`).
  `TestHelpGetConfigStagingTunnelDC` checks the single tunnel string. It is
  not a live `getConfig`. `TestParseMixedTunnelStillPublishesProduction`
  shows the server parser would publish both addresses, with
  `this_port_only`, if the staging gate did not refuse the mixed string.
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

The desktop then dials `127.0.0.1:10444`. That is MTProto TCP. Host `11444`
is WebSocket. The UI acceptance harness speaks TCP and refuses `11444`.
This does not change DNS or a security group. An address written as
`<public-ip>:10444` is not this loopback bind and is not isolation.

This path is feasible only if that SSH account allows local TCP forwarding.
The read-only pass did not check `AllowTcpForwarding`. Confirm that at
execution time. If it is off, stop. Do not publish the ports to `0.0.0.0`
in this step.

GitHub-hosted runners cannot use that tunnel. UI acceptance has no default
host. A self-hosted runner on the staging host can use `127.0.0.1` and
`10444` only after the evidence file below exists. `E2E_STAGING_CONFIRMED`
and the port are not that evidence. Do not point it at `api.tellomi.cn`
or port `10443`.

## Project isolation

Compose name precedence, checked with Compose 2.40.3 `config` (no daemon,
no `up`): `-p`, then `COMPOSE_PROJECT_NAME`, then the file `name:`, then
the directory basename. A directory named `tellomi` and no
`COMPOSE_PROJECT_NAME` used to render project `tellomi` and volume
`tellomi_kafka_data`. `down` on that project is production.

`docker-compose.staging.yaml` sets `name: tellomi-staging`, which covers
the directory-basename case. `COMPOSE_PROJECT_NAME=tellomi` still overrides
`name:` and renders project `tellomi`. The only lifecycle entry is
`scripts/staging-compose.sh`. It refuses that variable unless it is already
`tellomi-staging`, refuses `COMPOSE_FILE`, unsets both, and always passes
`-p tellomi-staging` plus the three compose files. `check` renders JSON
and exits non-zero unless the project, the Kafka bind, the default service
set, the loopback ports, and the memory caps match. `scripts/staging_compose_config_test.sh`
fails when the wrapper accepts the production project name.

Do not remember an extra `export`. Do not run `docker compose` for staging
from a shell that already has `COMPOSE_PROJECT_NAME` or `COMPOSE_FILE`.

## Bring-up, later

Do not run this until a parent review says to. This document does not
deploy.

```bash
export STAGING_ENV_FILE=.env.staging
scripts/staging-compose.sh check
scripts/staging-compose.sh up -d
```

Do not pass `--build`. Do not pass `docker-compose.limits.yaml`. The first
`up` creates `./data-staging` and loads SQL through the MySQL entrypoint and
`mysql-migrate`. It does not read `./data/mysql`. `down`, `ps`, `logs`,
`pull`, `stop`, `start`, and `rm` go through the same script.

## What can be checked, and what cannot

Two different things were easy to mix together.

A host capture is output from commands run on the host, read by a person
before any dial. This repository cannot collect it. The commands, when
someone with access runs them later, are `docker inspect` on
`tellomi-staging-teamgram` (name, port bindings, mounts), `ss` for
`127.0.0.1:10444`, and the Compose project name from
`scripts/staging-compose.sh ps`. None of those commands has been run for
this change. Their output is not in git.

An operator attestation is text a person types. The UI harness can reject
shapes that are obviously wrong. Passing that check does not authenticate
the host. GitHub Actions `workflow_dispatch` input `staging_evidence` is
ignored. The workflow sets `E2E_STAGING_EVIDENCE_SOURCE=workflow-input`,
and `redirect.py` refuses that source. It does not write the pasted text
to a file and then call the file a capture.

The harness dials only `127.0.0.1`. A public IP is refused even when the
attestation file contains the four lines below. Those lines have to be
exact lines, not substrings:

```text
container=tellomi-staging-teamgram
listen=127.0.0.1:10444
project=tellomi-staging
datadir=data-staging
```

A `mount=` line is path-normalized (`//`, `.`, `..`, and a missing trailing
slash). `/opt/tellomi/teamgram-server/data` and `/opt/tellomi/secrets`, and
anything under them, are refused. A missing `mount=` line does not prove
the mount is absent. The confirmation word and port `10444` are not proof
either. Host `11444` is WebSocket and this TCP harness refuses it.

This acceptance scope is desktop MTProto TCP `10444` only. iOS ignores
`this_port_only` and still probes `80` and `5222` (`pkg/dcoptions` and
`docs/ops/DC_ADDRESSES.md`). Before any later real acceptance, check each
desktop client's DC list and what it does when the SSH tunnel reconnects.
That check has not been done. This PR does not change a client.

## Keys and DC, not a passed interop

Empty extra-key variables load fingerprint `12240908862933197005` from
`teamgramd/etc2/gnetway.yaml`. Production clients pin
`4381149949997195609`. Those two fingerprints are not interchangeable.
gnetway startup logs redact the fingerprint (`startup_redact_test.go`), so
the log line is not the check. The rendered environment must keep both
`TELLOMI_MTPROTO_EXTRA_KEY_*` values empty, and must not mount
`/opt/tellomi/secrets`. `scripts/staging-compose.sh` fails `check` when
either value is set.

A desktop-only staging config would need that desktop build to trust the
public key for `12240908862933197005` on purpose. Copying the production
private key into staging is not that step, and this PR does not do it.
This PR also does not change the production client pin. Until a desktop
build has that explicit trust, and until someone has compared it with the
key the staging process actually loaded, do not describe staging as able
to log in. No such build and no such comparison exist here.

`help.getConfig` publishes the tunnel DC only when `.env.staging` is
exactly `TELLOMI_DC_OPTIONS=1:127.0.0.1:10444`. `up` parses that value and
refuses anything else before it calls Docker. The unit test
`TestHelpGetConfigStagingTunnelDC` checks the single string. It is not a
live `getConfig` against a running server.

## Acceptance and rollback

On the host, after the containers exist, and only then:

1. `docker ps` names start with `tellomi-staging-`. No staging container
   mounts `/opt/tellomi/teamgram-server/data`. Keep the inspect output.
   Retyping four lines from it is not a substitute for that output.
2. `ss` shows `127.0.0.1:10444` owned by `tellomi-staging-teamgram`.
   `10443`, `11443`, and `5222` stay on the production container.
3. TCP to `127.0.0.1:10444` succeeds on the host. Then
   `TELLOMI_E2E_ADDR=127.0.0.1:10444 ./scripts/smoke-e2e.sh` using the
   `99966` numbers in `e2e/client/config.json`. That script is not part of
   this change and has not been run against a staging stack here.
4. A desktop through the SSH forward is a later check. It is not claimed
   here. No production user is required for the `99966` range.

Rollback is `scripts/staging-compose.sh down` only. Deleting
`./data-staging` drops the new database, including Kafka, and nothing under
`./data`. Do not `FLUSHALL` the production Redis. There is no production
migration in this plan. Image tags `c81f90ff` and `168c6f58` are not
deployed by this document. The operator report of free disk is not
authorization to `up`.

# Datacenter addresses (M7 / D-33)

Installed iOS and Android clients learn a new server IP from `help.getConfig`
`dc_options`. They do not need an app update. This page is how that list is
set, checked, and taken back.

iOS behavior is [tellomi/Telegram-iOS PR #55](https://github.com/tellomi/Telegram-iOS/pull/55)
(`docs/dc-addresses.md`). Android stores the same list once
`TELLOMI_LOCK_DC_ADDRESSES` is false in `tellomi/Telegram`. That client change
is a separate PR. Android accepts only IPv4 literals (`inet_pton`).

## What ships in git

`teamgramd/bin/config.json` is copied to `/app/bin/config.json`. `bff` reads
it once, from `./config.json`, when the process starts. The checked-in file
is still:

| Field | Value |
| --- | --- |
| `dc_options` | `[]` |
| `dc_txt_domain_name` | `apv2.stel.com` |
| `webfile_dc_id` | `1` |
| `this_dc` | `1` |

`docker-compose.yaml` does not mount the file. With `TELLOMI_DC_OPTIONS` and
`TELLOMI_PUBLIC_DC_HOST` unset, `help.getConfig` returns that empty list.
Merging the env hook does not move production clients. Do not commit a
production IP into the file. A container started without the env would then
publish it forever.

A `dcOption` without `predicate_name: dcOption` is dropped by the encoder.
The list on the wire looks empty. The env hook always sets that predicate.

## Where a reply comes from

`help.getConfig` does not read a database. `bff` loads `./config.json` once
at process start, clones it on each call, then applies the environment.
Nothing else fills `dc_options`. The checked-in file is an empty list, so a
process with the variables unset publishes no address. This repository does
not contain the host `.env`, so it cannot show the value a running
production process is publishing. `8.156.40.105` and `8.137.198.240` are
not in `config.json`.

## Environment

`docker-compose.yaml` passes the variables into `teamgram`. Set them in the
host `.env` and recreate the container. Docker does not update a running
process environment. Each `help.getConfig` reads the variables, so a recreated
container picks them up without editing `config.json`.

### `TELLOMI_DC_OPTIONS`

Comma-separated `dcId:ipv4:port`. IPv4 literal only.

```
TELLOMI_DC_OPTIONS=1:8.137.39.82:10443,1:8.137.39.82:5222
```

| Rule | |
| --- | --- |
| DC id | `1`–`65535`, no leading zero, no `+` |
| Address | dotted IPv4, no leading zeros, no hostname, no IPv6 |
| Port | `1`–`65535`, no leading zero |
| Size | at most 32 entries, spec at most 2048 bytes |
| Spaces | trimmed around the spec and around each entry |

Unset, empty, or whitespace: `dc_options` and `webfile_dc_id` stay as loaded
from the file.

Invalid: one error log (`help.getConfig: TELLOMI_DC_OPTIONS ... is invalid`),
the file values stay, the process stays up. A later valid value is used on
the next call. The same invalid string is not logged again.

Valid: that list replaces `dc_options`, and `webfile_dc_id` becomes `1`.
DC 4 does not exist. `this_dc` stays `1`. The list should include DC 1.

Each entry on the wire:

| Flag | Value |
| --- | --- |
| `predicate_name` | `dcOption` |
| `ipv6` | false |
| `media_only` | false. A media-only address is skipped for the main connection. |
| `tcpo_only` | false. gnetway does not require it. |
| `cdn` | false |
| `static` | false |
| `this_port_only` | true |
| `secret` | omitted |

`this_port_only` is for clients that read it, including Android. They dial
the declared port. `10443` is not one of the ports those clients substitute
(`443`, `80`, `5222`). iOS ignores the flag. During discovery it still probes
`80` and `5222` in addition to the declared port. `5222` is open on the origin gnetway. `80` is not. `10443` has no extra 5
second delay, so it is chosen when it answers. A capture that shows `80` or
`5222` is that probe, not a rewritten address set. The public relay does not
forward `5222`, so that probe against the relay address does not connect.

The origin gnetway listens on `0.0.0.0:10443` and `0.0.0.0:5222` inside the
container, plus WebSocket `11443`. Those binds are not the address published
in `dc_options`. `11443` is not an MTProto `dcOption`. Do not put
`api.tellomi.cn` in the list.

### `TELLOMI_DC_TXT_DOMAIN`

| Value | Result |
| --- | --- |
| unset or empty | leave `apv2.stel.com` |
| `-` | blank `dc_txt_domain_name` |
| anything else | use that string |

Compose always passes the variable, so empty has to mean "leave the file".
`-` is the blank. iOS does not read this field (backup discovery is already
off). Android may still use it and would resolve Telegram's
`apv2.stel.com`. Blank it when publishing DC addresses. It is not blanked
automatically. An invalid `TELLOMI_DC_OPTIONS` still applies a domain change.

### `TELLOMI_PUBLIC_DC_HOST`

The IPv4 clients should dial. Read only while building this reply. It does
not change gnetway listen addresses, the private PROXY bind, etcd, MySQL,
Redis, or `TELLOMI_CALL_IP` / TURN.

| Value | Result |
| --- | --- |
| unset or empty | leave the addresses already chosen |
| canonical dotted IPv4 | replace a dotted IPv4 in `dc_options`. DC ids and flags on a kept entry stay. A hostname stays. An entry with the `ipv6` flag, or a literal that contains `:` (`2001:db8::1`, `::ffff:192.0.2.1`), stays. `dc_txt_domain_name` is not this field. |
| anything else | one error log (`help.getConfig: TELLOMI_PUBLIC_DC_HOST ... is invalid`), the list stays, the process stays up |

While the host is valid, an entry whose port is not `10443` or `443` is
removed from the published list. Every such call logs the dropped
`dc:ip:port` values
(`help.getConfig: TELLOMI_PUBLIC_DC_HOST dropped dc_options whose port is not 10443 or 443`).
The log is not suppressed after the first time. `5222` is not published.

A DC that still has `10443` or `443` keeps those entries. A DC whose every
entry was dropped falls back to that same DC on `10443` and `443`. The
fallback does not copy the address onto a different DC id. An empty input
(no DC id at all) is the only case that publishes DC 1 alone. Do not clear
`TELLOMI_DC_OPTIONS` when DC 2 must stay: an empty list does not keep DC 2.

`this_dc` and the file DC (the first id in `TELLOMI_DC_OPTIONS`, or 1
when that variable is empty or invalid) must each have a published `10443`
or `443` address before the rewrite is used. If either does not, the
rewrite is not applied, an error is logged, and the process keeps serving
the list already loaded. An invalid `TELLOMI_DC_OPTIONS` is the same:
the whole spec is refused, including a leading DC id `0`, and the public
host is not applied, so a later DC is not replaced by a synthesized DC 1.
A `dcOption` loaded from the file with DC id `0` is not published. Example:
options are only `2:<origin>:5222` and `this_dc` is 1. DC 1 is not invented
and the origin address stays in the reply.

`TELLOMI_DC_OPTIONS` still builds flags itself, and those entries have
`media_only` and `cdn` false. A `cdn` or `media_only` entry is rewritten
only when it was loaded from `config.json` and the options variable did not
replace the list. The checked-in file has neither.

`help.getCdnConfig` publishes RSA keys from `TELLOMI_CDN_PUBLIC_KEYS`. It
does not publish an IP. Empty keys mean there is no CDN address. Photos and
documents store `dc_id` from the first id in `TELLOMI_DC_OPTIONS`, or 1.
Clients resolve that id through `dc_options`, so they follow the rewritten
address. There is no second media host in this code.

The relay forwards `443` and `10443` only. A client that stores another
port on that host keeps a blackhole, so those entries are not published.

## What production should set

On the origin host `.env` (the machine behind the relay), then recreate
`teamgram`. Do not put this in git. Do not write it into `config.json`.

The list in production already has DC 1 and DC 2. Keep both. An empty
`TELLOMI_DC_OPTIONS` plus `TELLOMI_PUBLIC_DC_HOST` publishes DC 1 only.

```
TELLOMI_PUBLIC_DC_HOST=8.137.198.240
TELLOMI_DC_OPTIONS=1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443
TELLOMI_DC_TXT_DOMAIN=-
```

`8.137.198.240` is the public relay. It forwards TCP `443` and `10443` with
PROXY protocol to the origin. `8.156.40.105` and `10.20.0.218` stay off
this list. Clients that learn `8.156.40.105` dial the origin directly and
skip the relay.

Do not clear `TELLOMI_DC_OPTIONS`. Do not leave `5222`, `8.137.39.82`, or
`8.156.40.105` in it. A port other than `10443` or `443` is dropped. A DC
that then has no port falls back to that same DC, not to DC 1. The origin
container is not started by compose. Recreate it from its inspect, below.

`TELLOMI_DC_TXT_DOMAIN=-` is recommended so Android backup discovery does not
point at Telegram. Omit it, or leave it empty, to keep `apv2.stel.com`.
Leave `11443` out. It is WebSocket, not an MTProto `dcOption`.

`TELLOMI_CALL_IP` and the TURN host are UDP. This relay does not forward
them. Do not point calls at `8.137.198.240` from this change.

## Clients persist the list

Only deliver addresses that complete MTProto. A blackhole IP is stored.

iOS, logged in, calls `help.getConfig` at startup and about hourly. A
non-empty `dc_options` replaces the keychain set for each DC id
(`persistent:datacenterAddressSetById`). An empty list updates nothing, so a
bad empty config cannot wipe a stored list. The seed in
`Tellomi/tellomi-server.json` (`api.tellomi.cn:10443`, DC 1) is used only
while that DC's persisted set is empty. Nothing writes the seed back over a
non-empty set. After a failure, discovery appends the seed and can still
reach production, but every launch tries the stored IP first.

When the persisted set was empty, iOS resets the transport onto the new list
in that same session. When it already had a list, the new IP is used after
the next process start, or after the current connection fails.

Android will store the same payload once `TELLOMI_LOCK_DC_ADDRESSES` is
false. Until that client build, Android keeps its compiled-in address.

Changing the server IP later: publish the old and new addresses together,
wait through the overlap, then drop the old ones.

```
TELLOMI_DC_OPTIONS=1:<old>:10443,1:<old>:443,2:<old>:10443,2:<old>:443,1:<new>:10443,1:<new>:443,2:<new>:10443,2:<new>:443
TELLOMI_PUBLIC_DC_HOST=
```

Leave `TELLOMI_PUBLIC_DC_HOST` empty for that overlap. A set host rewrites
every dotted IPv4 in the list, so the old address would not be published.
Keep DC 2 on both addresses. Ports other than `10443` and `443` are dropped
while the public host is set.

## Test IP

Same check as Telegram-iOS PR #55. The test IP must speak MTProto (this
gnetway, or an address that forwards to it). Use the env var. Leave
`TELLOMI_PUBLIC_DC_HOST` empty while the test list is a different address,
or the host rewrite replaces it. Do not hand-edit `/app/bin/config.json`
in a running container: `bff` will not see a file change until restart, and
the next image recreate puts the empty file back. The env survives recreate.

1. Set `TELLOMI_DC_OPTIONS` to the test IP, DC 1, port `10443` (and `5222`
   if that port is open there). Example: `1:203.0.113.10:10443`. Recreate
   `teamgram`.
2. Read `help.getConfig` before using a phone. `dc_options` contains that
   IP and port, `predicate_name` is `dcOption`, `webfile_dc_id` is 1.
   An invalid spec leaves the previous list and logs the error above.
   The client-regression job does this with `1:127.0.0.1:10443,1:127.0.0.1:5222`
   (`TestHelpGetConfig` when `TELLOMI_E2E_DC_OPTIONS` is set). That address
   is the CI host, not Chengdu.
3. Open an already installed, logged-in iOS build. Do not upgrade it. It
   calls `help.getConfig` immediately. With an empty local set, that session
   reconnects to the new list.
4. Confirm TCP from the phone to the test IP on `10443`. Kill and relaunch.
   The phone still dials the test IP. The seed hostname is not written back.
5. iOS may also probe `80` and `5222`. That is discovery, not the stored
   port. `10443` is tried first, without the extra delay.
6. Clear `TELLOMI_DC_OPTIONS` (or put the real production list back) and
   recreate the container. Phones that stored the test IP keep it. To return
   a test phone to the seed, delete the app or clear the keychain key
   `persistent:datacenterAddressSetById`. If the test IP stops answering,
   failed discovery still tries `api.tellomi.cn`.

## Roll back

Do not clear `TELLOMI_DC_OPTIONS`. Production publishes DC 2 from that
variable. Clearing it while `TELLOMI_PUBLIC_DC_HOST` is set replaces the
reply with DC 1 only. Phones that already stored an address keep it. Plan
the overlap above before withdrawing an IP. Do not `FLUSHALL` Redis. The
origin container is recreated from its inspect, not from compose.

## 上线与回滚（240 转发）

105 上的 `tellomi-main-teamgram` 不是 compose 服务，没有 compose 标签。
`/opt/tellomi/teamgram-server` 在这台机器上不存在。不要执行
`docker compose up`，那会再起一个容器去抢端口，并且不会带上
`turn-relay.env` 里的环境变量。

以这台机器上正在运行的容器的 `docker inspect` 为准。下面的脚本只改三个
数据中心环境变量，其余环境变量、挂载、网络、端口绑定（包括私网
`10445/tcp`）都从这份 inspect 原样复制。inspect 里没有 `10445/tcp` 时脚本
拒绝重建。脚本不打印环境变量的值。

先把本提交里的三个文件拷到固定路径（用 scp 或别的方式，不要假设仓库目录
已经在 105 上）：

- `scripts/ops/recreate-teamgram-dc.sh` → `/var/backups/recreate-teamgram-dc.sh`
- `scripts/ops/recreate_teamgram_from_inspect.py` → `/var/backups/recreate_teamgram_from_inspect.py`
- `scripts/ops/help-getconfig-dc.py` → `/var/backups/help-getconfig-dc.py`

上线：

```bash
set -euo pipefail
umask 077
chmod 755 /var/backups/recreate-teamgram-dc.sh /var/backups/recreate_teamgram_from_inspect.py /var/backups/help-getconfig-dc.py
/var/backups/recreate-teamgram-dc.sh apply
```

`apply` 自己会调用 `help.getConfig`，等 bff 起来（大约 8 秒，最多重试约
30 秒）。健康标准是读回的四条，顺序不限：

```
1 8.137.198.240 10443 ipv6=0
1 8.137.198.240 443 ipv6=0
2 8.137.198.240 10443 ipv6=0
2 8.137.198.240 443 ipv6=0
```

密钥在容器里的 `/app/bin/server_pkcs1.key`。脚本用 `--workdir /app/bin`
只取出公钥。对不上这四条，或调用失败，`apply` 会自动用
`tellomi-main-teamgram.dc-bak` 回滚。不要只看容器是不是 Running，也不要
只看 `printenv`。`runall` 末尾的空 wait 会让 bff 退出后容器仍显示 Running。

每次运行 `help-getconfig-dc.py` 都会在服务器上新建一把未登录的 auth key。
它不写用户、不打验证码。不要把它放进定时任务。

inspect 和 create JSON 含全部环境变量，权限是 `0600`。

没有 `5222`，没有 `8.156.40.105`，没有 `10.20.0.218`。

`apply` 会把当前容器改名为 `tellomi-main-teamgram.dc-bak`。这个名字是固定
的。备份容器还在时脚本拒绝再次 apply，避免覆盖备份。配置若让 `this_dc`
或文件 DC 没有地址，bff 不会退出：它记下错误并继续用改写前的
`dc_options`。所以上面的四条对不上时才会回滚。

不要清空 `TELLOMI_DC_OPTIONS`。空列表只会生成 DC 1。DC id `0` 会让整段
`TELLOMI_DC_OPTIONS` 失效，改写不会启用。生产列表必须保留 DC 2。
不要 `FLUSHALL`。

回滚：

```bash
set -euo pipefail
/var/backups/recreate-teamgram-dc.sh rollback
/var/backups/help-getconfig-dc.py --host 127.0.0.1 --port 10443 --workdir /app/bin
```

`rollback` 先检查 `tellomi-main-teamgram.dc-bak` 在、镜像名非空、状态是
created、running、exited 或 paused。不满足就直接退出，不停止、不改名生产
容器。检查通过后才停当前容器，把备份改回 `tellomi-main-teamgram` 并
start。再跑一次读取脚本会再建一把未登录 auth key。读回来的
`help.getConfig` 应与 apply 之前一致。已经存下地址的手机会继续先拨那个地址。
要让它们离开 240，先按上面的重叠列表同时公布新旧地址。发现失败时客户端仍会
试种子域名 `api.tellomi.cn`。

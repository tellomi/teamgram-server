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
removed from the published list. The process does not exit. One error log
names the dropped `dc:ip:port` values
(`help.getConfig: TELLOMI_PUBLIC_DC_HOST dropped dc_options whose port is not 10443 or 443`).
`5222` on the relay is not published. The same string is not logged again.

A valid host on an empty list (the checked-in file, and `TELLOMI_DC_OPTIONS`
unset) publishes two DC 1 options only: that host on `10443` and on `443`,
with `this_port_only`, not `media_only`, not `cdn`. `webfile_dc_id` becomes
1. It does not invent DC 2. A list whose every entry was dropped, such as
one that contains only `5222`, becomes that same DC 1 pair. Do not clear
`TELLOMI_DC_OPTIONS` when DC 2 must stay.

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
`8.156.40.105` in it. A port other than `10443` or `443` is dropped from
the reply when the public host is set, and a list that becomes empty is
replaced by DC 1 only.

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
the overlap above before withdrawing an IP. Do not `FLUSHALL` Redis. Do
not change gnetway binds or the private PROXY listener. The commands are
in the next section.

## 上线与回滚（240 转发）

对外公布的地址和内部监听地址不是同一个配置。客户端看到的是
`help.getConfig` 的 `dc_options`。gnetway 仍监听容器里的 `0.0.0.0:10443`。
不要改 etcd、MySQL、Redis、`TELLOMI_CALL_IP`，也不要 `FLUSHALL`。

生产当前的 `TELLOMI_DC_OPTIONS` 是下面这一行，包含 DC 1 和 DC 2。空列表
只会生成 DC 1 的 `10443` 和 `443`。不要清空这个变量。

`TELLOMI_PUBLIC_DC_HOST` 生效时，端口不是 `10443` 或 `443` 的条目会从下发
列表里丢掉，并打一条错误日志，进程继续跑。`::ffff:x.x.x.x` 和带 `ipv6`
标志的地址不改写。

在 origin 上执行。工作目录是检出仓库的目录，成都是
`/opt/tellomi/teamgram-server`。

上线：

```bash
cd /opt/tellomi/teamgram-server
stamp=$(date -u +%Y%m%dT%H%M%SZ)
cp -a .env ".env.bak.${stamp}"
python3 - <<'PY'
from pathlib import Path
p = Path(".env")
text = p.read_text()
want = {
    "TELLOMI_PUBLIC_DC_HOST": "8.137.198.240",
    "TELLOMI_DC_OPTIONS": "1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443",
    "TELLOMI_DC_TXT_DOMAIN": "-",
}
seen = set()
out = []
for line in text.splitlines():
    key, sep, _ = line.partition("=")
    if sep and key in want:
        out.append(key + "=" + want[key])
        seen.add(key)
    else:
        out.append(line)
for key, value in want.items():
    if key not in seen:
        out.append(key + "=" + value)
p.write_text("\n".join(out) + "\n")
PY
C="docker compose -p tellomi -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml -f docker-compose.tellomi.yaml"
$C up -d --no-deps --force-recreate teamgram
$C exec -T teamgram sh -c 'test "$TELLOMI_PUBLIC_DC_HOST" = "8.137.198.240" && test "$TELLOMI_DC_OPTIONS" = "1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443"'
echo "backup=.env.bak.${stamp}"
```

`test` 非 0 就不要继续。把备份拷回去并重建：

```bash
cp -a ".env.bak.${stamp}" .env
$C up -d --no-deps --force-recreate teamgram
```

验证通过后，用已登录客户端读 `help.getConfig`。应有四条 `dcOption`：DC 1
和 DC 2，各有 `10443` 和 `443`，地址都是 `8.137.198.240`，没有 `5222`，
没有 `8.156.40.105`，没有 `10.20.0.218`。`help.getCdnConfig` 在
`TELLOMI_CDN_PUBLIC_KEYS` 为空时没有公钥，也没有 IP。

回滚只清掉对外改写，保留含 DC 2 的列表：

```bash
cd /opt/tellomi/teamgram-server
stamp=$(date -u +%Y%m%dT%H%M%SZ)
cp -a .env ".env.bak.${stamp}"
python3 - <<'PY'
from pathlib import Path
p = Path(".env")
text = p.read_text()
out = []
seen = False
for line in text.splitlines():
    key, sep, _ = line.partition("=")
    if sep and key == "TELLOMI_PUBLIC_DC_HOST":
        out.append("TELLOMI_PUBLIC_DC_HOST=")
        seen = True
    else:
        out.append(line)
if not seen:
    out.append("TELLOMI_PUBLIC_DC_HOST=")
p.write_text("\n".join(out) + "\n")
PY
C="docker compose -p tellomi -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml -f docker-compose.tellomi.yaml"
$C up -d --no-deps --force-recreate teamgram
$C exec -T teamgram sh -c 'test -z "$TELLOMI_PUBLIC_DC_HOST" && test "$TELLOMI_DC_OPTIONS" = "1:8.137.198.240:10443,1:8.137.198.240:443,2:8.137.198.240:10443,2:8.137.198.240:443"'
echo "backup=.env.bak.${stamp}"
```

验证失败时用同一条 `cp -a ".env.bak.${stamp}" .env` 再重建 `teamgram`。
已经存下地址的手机会继续先拨那个地址。要让它们离开 240，先按上面的重叠
列表同时公布新旧地址，等客户端拿到新列表，再撤掉旧的。发现失败时客户端
仍会试种子域名 `api.tellomi.cn`。

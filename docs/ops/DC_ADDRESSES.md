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
| `webfile_dc_id` | `4` |
| `this_dc` | `1` |

`docker-compose.yaml` does not mount the file. With `TELLOMI_DC_OPTIONS`
unset, `help.getConfig` returns that empty list. Merging the env hook does
not move production clients. Do not commit a production IP into the file. A
container started without the env would then publish it forever.

A `dcOption` without `predicate_name: dcOption` is dropped by the encoder.
The list on the wire looks empty. The env hook always sets that predicate.

## Environment

`docker-compose.yaml` passes both variables into `teamgram`. Set them in the
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
`80` and `5222` in addition to the declared port. `5222` is open on gnetway.
`80` is not. `10443` has no extra 5 second delay, so it is chosen when it
answers. A capture that shows `80` or `5222` is that probe, not a rewritten
address set.

gnetway on production listens on `8.137.39.82` TCP `10443` and `5222`, plus
WebSocket `11443`. `11443` is not an MTProto `dcOption`. Do not put
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

## What production should set

On the Chengdu host `.env`, then recreate `teamgram`. Do not put this in git.

```
TELLOMI_DC_OPTIONS=1:8.137.39.82:10443,1:8.137.39.82:5222
TELLOMI_DC_TXT_DOMAIN=-
```

Both TCP ports are what gnetway already accepts. Publishing only `10443` is
enough for the Tellomi seed port. Publishing `5222` as well gives a client
that prefers the classic port an address that answers. Leave `11443` out.

`TELLOMI_DC_TXT_DOMAIN=-` is recommended so Android backup discovery does not
point at Telegram. Omit it, or leave it empty, to keep `apv2.stel.com`.

Staging does not use that production list. `scripts/staging-compose.sh up`
accepts only `TELLOMI_DC_OPTIONS=1:127.0.0.1:10444`. The server parser would
publish a mixed list such as `1:8.137.39.82:10443,1:127.0.0.1:10444` as two
`this_port_only` addresses, and Android would dial the production one. iOS
still probes `80` and `5222` because it ignores `this_port_only`. The
current staging acceptance scope is desktop MTProto TCP `10444` only. See
[STAGING_BRINGUP.md](STAGING_BRINGUP.md).

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
TELLOMI_DC_OPTIONS=1:<old>:10443,1:<old>:5222,1:<new>:10443,1:<new>:5222
```

## Test IP

Same check as Telegram-iOS PR #55. The test IP must speak MTProto (this
gnetway, or an address that forwards to `8.137.39.82:10443`). Use the env
var. Do not hand-edit `/app/bin/config.json` in a running container: `bff`
will not see a file change until restart, and the next image recreate puts
the empty file back. The env survives recreate.

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

1. Remove `TELLOMI_DC_OPTIONS` from `.env`, or set it empty. Leave
   `TELLOMI_DC_TXT_DOMAIN` empty to restore `apv2.stel.com`.
2. Recreate `teamgram`. Do not `FLUSHALL` Redis.
3. `help.getConfig` returns `dc_options: []` and `webfile_dc_id: 4` again,
   matching the file in the image.
4. Phones that already adopted the list do not move back. Plan the overlap
   in the previous section before withdrawing an IP those phones stored.

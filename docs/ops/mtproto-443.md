# Plain MTProto on TCP/443

Production TCP/443 stays the existing host nginx stream. TLS keeps the
current SNI routes. A connection that is not TLS is plain obfuscated
MTProto and is forwarded to a new gnetway listener that accepts PROXY v1.
`10443`, `5222`, and `11443` are unchanged and do not accept PROXY v1.
TCP/80 is unchanged. No new public port is opened. `help.getConfig`
`dc_options` is not changed in this change: the Android client adds 443
itself.

## Clients

The Android client (separate, unmerged) dials the raw IP `8.137.39.82`
port `443` with no SNI and sends bare obfuscated MTProto. It tries
`10443` first, waits about 6 seconds, then tries `443`. The non-TLS route
is chosen only because `$ssl_preread_protocol` is empty. It is not chosen
by name.

iOS requirements for this port are still pending. This document does not
describe an iOS build.

## gnetway

`teamgramd/etc2/gnetway.yaml` (and the non-Docker `teamgramd/etc/gnetway.yaml`)
add a separate TCP group:

```yaml
- Proto: tcp
  PPV1: true
  Addresses:
    - 0.0.0.0:10444
```

The existing groups stay `0.0.0.0:10443` and `0.0.0.0:5222` with PPV1 off,
and websocket `0.0.0.0:11443` with PPV1 off. `0.0.0.0` here is the address
inside the container, not a host publish.

`IsProxyProtocolV1` compares that string exactly. gnet copies the
listening socket's address onto each accepted connection
(`acceptor_unix.go` passes `listeners[fd].addr` into `newStreamConn`).
That address is `net.ResolveTCPAddr` of the configured string, not
`getsockname` of the accepted fd. A client that dials `127.0.0.1:10444`
still has `LocalAddr` `0.0.0.0:10444`, which is what `OnOpen` passes to
`IsProxyProtocolV1`. `127.0.0.1:10444` does not match. `0.0.0.0:10443`
does not match.

PROXY v1 is optional on that listener only: a connection whose first
bytes are not `PROXY ` continues as a normal MTProto transport. There is
no source-CIDR check, so the listener must not be reachable except from
the host nginx. Public `10443` does not parse a PROXY header. A header
sent there is ordinary payload and is not used as the client IP.

When the header is accepted, gnetway logs `ppv1 accepted client_ip=<ip>
local=0.0.0.0:10444`. The log line is the parsed address only. The bytes
after the header are the obfuscated init, and that init contains the
ephemeral transport key, so the raw buffer is not logged.

## Compose

`docker-compose.yaml` publishes:

```yaml
- "127.0.0.1:10444:10444"
```

That is host loopback to container `10444`. It is not `"10444:10444"`.
`e2e/docker-compose.loopback.yaml` includes the same line because that
overlay replaces the whole port list.

`docker-compose.tellomi.yaml` is the Chengdu host override and is not in
this repo. `deploy-pull.sh` always merges it. If that file sets
`services.teamgram.ports: !override`, the line in `docker-compose.yaml`
is dropped until the host file lists it too. Staging is a different
mapping and is not this listener: `docker-compose.staging.yaml` publishes
host port `10444` to container port `10443`. Do not point nginx at the
staging host port and expect PROXY v1.

Before recreating `teamgram`, from `/opt/tellomi/teamgram-server`:

```bash
ss -lnt | grep 10444 || true
C="docker compose -p tellomi -f docker-compose-env.yaml -f docker-compose.yaml -f docker-compose.geoip.yaml -f docker-compose.tellomi.yaml"
$C config --format json | python3 -c '
import json,sys
doc=json.load(sys.stdin)
hits=[p for p in doc["services"]["teamgram"].get("ports",[]) if str(p.get("target")) in ("10444","10444.0")]
print(hits)
ok=len(hits)==1 and hits[0].get("host_ip")=="127.0.0.1" and str(hits[0].get("published")) in ("10444","10444.0")
sys.exit(0 if ok else 1)
'
```

The python snippet prints only the 10444 publish entry. If it fails
because the override replaced ports, add this line to that override's
teamgram port list and run the command again:

```yaml
- "127.0.0.1:10444:10444"
```

Do not add `"10444:10444"`. After recreate, from another host,
`8.137.39.82:10444` must refuse the connection. `127.0.0.1:10444` on the
Chengdu host must accept it. This change does not open a security-group
port. `443/tcp` is already open. `10443/tcp` stays as it is.

## nginx

Tracked template: `deploy/nginx/chengdu-stream.conf`.

It keeps the current SNI map (`grpc.chat.tellomi.cn` to `127.0.0.1:8443`,
everything else to `127.0.0.1:4443`) and adds:

```nginx
map $ssl_preread_protocol $tellomi_entry_upstream {
    ""      127.0.0.1:10444;
    default $tellomi_upstream;
}
```

`proxy_protocol on` stays, so both the website backends and gnetway still
see the client address. The public `listen 443` does not set
`proxy_protocol` on the listen socket, so a client cannot supply its own
PROXY header to nginx.

### ssl_preread wait, and what a timeout does

Read from nginx `release-1.24.0` (the Chengdu package is
`1.24.0-2ubuntu7.19`; the CI image is `nginx:1.24.0`).

`src/stream/ngx_stream_ssl_preread_module.c`, `ngx_stream_ssl_preread_handler`:

1. With fewer than 5 bytes it returns `NGX_AGAIN`.
2. Legacy SSLv2 ClientHello heuristic: high bit of byte 0 set, byte 2 is
   1, byte 3 is 0 or 3. That returns `NGX_OK` immediately.
3. Otherwise byte 0 not `0x16`, or byte 1 not `0x03`, returns
   `NGX_DECLINED` and clears the module context. `$ssl_preread_protocol`
   is empty, and the map sends the connection to `127.0.0.1:10444`.
4. Bytes `16 03` are a TLS record. The handler waits until it has the
   record length from bytes 3 and 4, plus 5. A random obfuscated init is
   about 64 bytes plus one frame. A random length is usually larger, so
   the handler returns `NGX_AGAIN` and waits. The chance of the first two
   bytes being `16 03` is about 1/65536 (Android's blacklist removes a few
   values, so slightly higher). It is not 1/256.

`src/stream/ngx_stream_core_module.c`, `ngx_stream_core_preread_phase`:

```c
if (c->read->timedout) {
    rc = NGX_STREAM_OK;
}
```

`NGX_STREAM_OK` is 200 (`ngx_stream.h`). `NGX_OK` is 0 (`ngx_core.h`).
The same function advances to `proxy_pass` only when `rc == NGX_OK`.
200 does not match, so it calls `ngx_stream_finalize_session(s, rc)`.

`src/stream/ngx_stream_handler.c`, `ngx_stream_finalize_session`:

```c
s->status = rc;
ngx_stream_log_session(s);
ngx_stream_close_connection(s->connection);
```

The client connection is closed. The bytes already read are not sent to
`10444` and not sent to the HTTPS backends. A lower timeout does not turn
a TLS-lookalike init into a successful MTProto connection. It only closes
sooner. The client must open a new connection. A new random init is almost
never `16 03` again.

`preread_timeout` in the template is `5s` (the nginx default is `30s`).
A normal ClientHello is one record and is classified as soon as that
record has arrived, so 5s is not a delay on healthy HTTPS. It is the cap
on a record that never completes. A ClientHello that is still incomplete
after 5 seconds is closed too. That is the tradeoff against holding a
worker connection for 30 seconds on the rare MTProto collision. This
change does not change `worker_connections`.

The SSLv2 heuristic is a smaller false positive. Some of those random
inits are labeled TLS and go to the HTTPS backend, which then fails the
handshake. Reconnect with a new init is the same recovery. A classifier
that understands the obfuscated init would be a different change.

## Deploy

Do this only after the image that contains the `10444` listener is the
one `teamgram` is running, and the compose check above succeeded.
`nginx -t` must pass before reload. Do not edit port 80, `4443`, `8443`,
certificates, or the security group.

```bash
sudo cp -a /etc/nginx/stream.d/tellomi-sni.conf \
  /etc/nginx/stream.d/tellomi-sni.conf.bak-mtproto443
sudo cp deploy/nginx/chengdu-stream.conf /etc/nginx/stream.d/tellomi-sni.conf
sudo nginx -t
sudo systemctl reload nginx
```

Then, on the host, an obfuscated abridged probe to the raw IP with no SNI
(this script is not TLS, so it cannot send a server name):

```bash
python3 scripts/mtproto-probe.py 8.137.39.82 443 --avoid-preread-wait
python3 scripts/mtproto-probe.py 8.137.39.82 10443 --avoid-preread-wait
python3 scripts/mtproto-probe.py 127.0.0.1 10444 --proxy-v1-src 203.0.113.77 --avoid-preread-wait
```

`203.0.113.77` is the documentation range, used only so the gnetway log
can be told apart from the socket peer. Expect
`ppv1 accepted client_ip=203.0.113.77 local=0.0.0.0:10444` in
`/app/logs/gnetway` inside the container. A PROXY header sent to public
`10443` must not produce `resPQ` and must not log that source address.

A direct init prefixed with `160300ffff` completes `resPQ` on `10443` and
is closed by nginx on `443` after about 5 seconds without `resPQ`. That
is the preread wait, not a gnetway failure.

Repeat the existing HTTPS checks (`curl --http2` per name, port 80
redirects). Known failures that predate this change (chat, storage, cdn
root `502`, grpc TLS EOF) are not fixed here and are not a new regression
by themselves. API root `404` remains the baseline.

`nginx` reload does not move an already established connection. New
connections follow the new map. Rolling back the file does not cut those
old connections either.

## Rollback

`10443` is still the direct listener. Restoring the previous stream file
returns `443` to TLS-only:

```bash
sudo cp -a /etc/nginx/stream.d/tellomi-sni.conf.bak-mtproto443 \
  /etc/nginx/stream.d/tellomi-sni.conf
sudo nginx -t && sudo systemctl reload nginx
```

If the private listener itself is the problem, redeploy the previous
image and remove the `127.0.0.1:10444:10444` line from the host override
if it was added there. Do not roll back MySQL, Redis, or session data.
Do not `FLUSHALL`. Delete only the backup created above, and only after
the new file has been accepted.

## CI

`e2e/client` `TestMtproto443Stream` runs `nginx:1.24.0` with this template
on `127.0.0.1:18443` in front of the compose gnetway. It dials that
address as an IP literal with no SNI. It checks obfuscated abridged
`req_pq_multi` / `resPQ` through nginx, the PROXY v1 address in the
gnetway log, HTTP/2 with the matching certificate for
`www.tellomi.cn` and `grpc.chat.tellomi.cn`, a direct `10443` handshake,
and a PROXY header on `10443` that is not trusted. The `160300ffff` init
through nginx must fail without `resPQ`.

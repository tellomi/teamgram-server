# Private PROXY listener

An optional gnetway TCP listener accepts one PROXY protocol header from a
configured peer and stores that header's source address in the existing
`client_ip` field. `10443`, `5222`, `11443`, and the loopback `10444` PPV1
listener are unchanged. Nothing in the checked-in yaml turns the listener on.

## Config

Add a separate `Gnetway.Server` group. `ProxyProtocol` empty means the group
is not this listener. A non-empty value without `TrustedProxyCIDRs` makes
gnetway refuse to start. Each CIDR must be one IPv4 `/32` or one IPv6 `/128`.
The bind address must be one interface address, not `0.0.0.0` or `::`.

```yaml
- Proto: tcp
  ProxyProtocol: v1v2
  RequireProxyHeader: true
  TrustedProxyCIDRs:
    - 172.19.44.222/32
  Addresses:
    - 10.20.0.218:10445
```

`ProxyProtocol` is `v1`, `v2`, or `v1v2`. `v1v2` accepts PROXY v1 and v2,
TCP4 and TCP6. The header read times out after 3 seconds. A partial header,
a header longer than the library limit, a second header, LOCAL, UDP, UNIX,
or a connection whose socket peer is outside `TrustedProxyCIDRs` is closed.
The address inside the header is not trusted until the socket peer is.

`RequireProxyHeader` defaults to true. Leave it true for the relay.

Do not combine `ProxyProtocol` with `PPV1` on the same group. `10444` stays
`PPV1: true`, optional v1, no CIDR, published only on `127.0.0.1`.

The header source is written with `setClientIp` into the same string that
`initConnection` stores in `auths.client_ip` and that `send_code_ip` /
`sign_in_ip` use as the bucket key. There is no new column.

## Entry counters

gnetway counts accepted native connections and their later socket bytes.
The Prometheus names are `gnetway_entry_conn_total` and
`gnetway_entry_bytes_total`. The only class label is one of `direct_10443`,
`relay_10445`, `relay240_443`, or `relay_unknown`. `dir` on the byte counter
is `in` or `out`. There is no address, auth key, session id, or user id.

`direct_10443` is a connection accepted on port 10443. A PROXY header on
that socket is not read and does not change the class. `relay_10445` is a
header accepted from a trusted peer whose destination port is 10443 (the
240:10443 frontend, forwarded to this private listener). `relay240_443` is
the same admission whose destination port is 443. Any other destination
port is `relay_unknown`. The three accepted classes do not overlap, so they
must not be added to the older proxy-admit log lines. Bytes start after the
PROXY header is removed. `gnetway_entry_reject_total{reason}` counts closes
that never become a connection; `reason` is a fixed token, not the peer.

## Before enabling it

Confirm the process can bind `10.20.0.218` and that an accepted connection's
socket peer is `172.19.44.222`, not a bridge or SNAT address. If it is not
that `/32`, the listener closes the connection. Do not widen the CIDR to a
docker or VPC range. Do not publish the port on `0.0.0.0`. This file does
not change compose or open a security-group port.

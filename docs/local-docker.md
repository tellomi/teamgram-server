# Local deployment (Docker Compose)

Phase 1 target: a stock Telegram client on API layer 232 can register with a
phone number, log in, sync contacts, and send private text and photo messages.
This stack is the local way to run that server. It is a development setup.
The SMS provider accepts a fixed code and must not be used in production.

## What you get

| Piece | Role |
|---|---|
| `docker-compose-env.yaml` | MySQL, Redis, etcd, Kafka, MinIO, and the optional monitoring stack. First start loads SQL from `teamgramd/deploy/sql` and creates the `tellomi-attachments`, `tellomi-public-media`, and `tellomi-backups` buckets. |
| `docker-compose.yaml` | The Teamgram process image (gateway, session, bff, biz, msg, media, …). |
| `teamgramd/etc2/bff.yaml` | Config the container actually runs (`runall-docker.sh`). |

MTProto is served on **10443** (TCP) and **11443** (WebSocket). `5222` is an
additional TCP listener. The API layer is **232** (`proto/README.md`).

## Prerequisites

Docker with Compose v2. The image builds with Go 1.23 and needs network
access to module proxies on the first build. FFmpeg is installed in the
runtime image for media.

## Start

From the repository root:

```bash
cp .env.example .env   # optional; defaults match the compose file
docker compose -f docker-compose-env.yaml -f docker-compose.yaml up -d --build
```

That one command starts the dependencies and the app. `teamgram` waits until
Kafka's healthcheck passes. Starting the two files as separate projects still
works: msg, sync, and bff retry Kafka, MySQL, Redis, and etcd on their own.

```bash
docker compose -f docker-compose-env.yaml up -d
docker compose up -d --build
```

Wait until MySQL is healthy and `teamgram` stays up (`docker compose ps`).
Logs from the processes are inside the app container (`/app/logs`).

`--build` is for this machine. The Chengdu server must not compile the image.
It pulls the image GitHub Actions publishes. See [ci-acr.md](ci-acr.md).

## Phone login (fixed code)

`teamgramd/etc2/bff.yaml` sets:

```yaml
Code:
  Name: "none"
  FixedCode: "12345"
```

`none` does not send SMS. `auth.sendCode` still returns a `phone_code_hash`.
`auth.signIn` / `auth.signUp` accept **12345** for any number unless you
change `FixedCode`. Restart the `teamgram` container after editing the file
mounted into the image (the yaml is copied at build time, so rebuild after
changing it: `docker compose up -d --build`).

Use E.164 numbers without extra spaces, for example `+15551212001`.

## Point a client at it

Stock layer-232 clients (Telegram iOS `fb0e62ba`, Telegram Android `3de37c8`,
rebranded) should use this server’s DC address and the public key in
`teamgramd/bin/server_pkcs1.key` (fingerprint `12240908862933197005`). The
client build replaces Telegram’s production DC list and RSA key; that
rebrand is outside this repository. TCP port **10443** is the abridged /
intermediate / padded-intermediate listener.

## Prove the layer 232 codec

Docker is not required for the protocol test. It encodes and decodes the
objects those flows exchange at layer 232 (authorization, user, userFull,
imported contacts, text message, photo message, reply headers, channel):

```bash
cd proto && go test ./mtproto -count=1 -run 'TestLayer232'
```

From the server module, this checks the `replace` directive points at that fork:

```bash
go test ./clients/layer232 -count=1
```

A full handshake against the compose stack (req_pq through sendMessage) is
not automated here. The codec test is the check that a layer-232
`invokeWithLayer` no longer comes back as constructor 0.

## Object storage and RTMP

Object storage is S3-compatible. Local compose keeps MinIO. Production can
point the same variables at Cloudflare R2. The variable list, key prefixes,
and the R2 settings (region `auto`, path-style) are in [storage.md](storage.md).

`phone.getGroupCallStreamRtmpUrl` returns `TELLOMI_RTMP_URL` plus a stream
key. The default is `rtmp://127.0.0.1:1935/live`, which is the MediaMTX
service in `docker-compose.yaml`. Set `TELLOMI_RTMP_URL` when clients must
push to a public ingest host. The stream is not bridged into the group-call
SFU (`app/bff/phone/LIMITATIONS.md`).

## Stop

```bash
docker compose down
docker compose -f docker-compose-env.yaml down
```

Named volumes and `./data` keep MySQL, Kafka, and MinIO state. Remove those
directories if you want a clean database. Do not commit `.env` or `data/`.

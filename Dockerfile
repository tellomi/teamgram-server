# ip2region xdb (v4 and v6) from the official v3.18.0 tag. The script checks
# deploy/ip2region/SHA256SUMS and fails the image build on mismatch.
# See docs/ip2region.md.
FROM ubuntu:latest AS ip2region
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY scripts/fetch-ip2region-xdb.sh deploy/ip2region/SHA256SUMS /src/
RUN chmod +x /src/fetch-ip2region-xdb.sh \
    && /src/fetch-ip2region-xdb.sh /out \
    && rm -f /out/.fetch.lock

FROM golang:1.23.0 AS builder
WORKDIR /app
COPY . .
RUN ./build.sh
# Sticker seed tool. Assets are embedded in the binary. Lands at /app/bin/importstickers.
RUN cd app/bff/stickers/cmd/importstickers && go build -o /app/teamgramd/bin/importstickers
# Official account avatar seed tool (docs/official-avatars.md). Lands at /app/bin/seedofficialavatars.
RUN cd app/bff/userchannelprofiles/officialavatars/cmd/seedofficialavatars && go build -o /app/teamgramd/bin/seedofficialavatars

FROM ubuntu:latest
RUN apt update -y && apt install -y ffmpeg psmisc default-mysql-client && apt-get clean
WORKDIR /app
COPY --from=builder /app/teamgramd/ /app/
COPY --from=ip2region /out/ /app/data/ip2region/
# Names come from entrypoint.sh and the compose command/entrypoint values,
# not from a fixed list. A missing or non-executable file fails the build,
# so the workflow cannot push a partial image.
COPY scripts/verify-teamgram-binaries.sh /tmp/verify-teamgram-binaries.sh
COPY docker-compose.yaml docker-compose-env.yaml docker-compose.staging.yaml docker-compose.geoip.yaml /tmp/compose/
RUN chmod +x /app/docker/entrypoint.sh /app/deploy/migrate.sh /app/bin/importstickers /app/bin/seedofficialavatars /app/bin/runall-docker.sh \
    && bash /tmp/verify-teamgram-binaries.sh /app /tmp/compose \
    && rm -rf /tmp/verify-teamgram-binaries.sh /tmp/compose
# Exec form so the entrypoint is PID 1. Shell form wraps it in `sh -c`,
# which swallows SIGTERM and leaves client sockets open across a restart.
ENTRYPOINT ["/app/docker/entrypoint.sh"]

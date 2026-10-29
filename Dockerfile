FROM golang:1.23.0 AS builder
WORKDIR /app
COPY . .
RUN ./build.sh
# Sticker seed tool. Assets are embedded in the binary. Lands at /app/bin/importstickers.
RUN cd app/bff/stickers/cmd/importstickers && go build -o /app/teamgramd/bin/importstickers

FROM ubuntu:latest
RUN apt update -y && apt install -y ffmpeg psmisc default-mysql-client && apt-get clean
WORKDIR /app
COPY --from=builder /app/teamgramd/ /app/
RUN chmod +x /app/docker/entrypoint.sh /app/deploy/migrate.sh /app/bin/importstickers /app/bin/runall-docker.sh
# Exec form so the entrypoint is PID 1. Shell form wraps it in `sh -c`,
# which swallows SIGTERM and leaves client sockets open across a restart.
ENTRYPOINT ["/app/docker/entrypoint.sh"]

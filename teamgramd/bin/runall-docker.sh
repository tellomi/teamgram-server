#!/usr/bin/env bash
# Logs go to /app/logs/*.log (volume) for filebeat; works on Docker Desktop Mac/Win where
# /var/lib/docker/containers is not visible to filebeat container.
#
# This script is PID 1 (entrypoint execs it). It has to catch SIGTERM and
# forward it. Otherwise Docker kills the container without gnetway closing
# client sockets, and clients sit on the half-open connection for the 300s
# idle timer.

pids=()

start() {
  echo "run $1 ..."
  shift
  "$@" &
  pids+=($!)
}

shutdown() {
  echo "stopping teamgram processes"
  # Reverse order: gnetway is near the end, so it is signalled first and can
  # FIN client connections before the other processes exit.
  for ((i=${#pids[@]}-1; i>=0; i--)); do
    kill -TERM "${pids[i]}" 2>/dev/null || true
  done
  deadline=$((SECONDS + 12))
  while (( SECONDS < deadline )); do
    alive=0
    for pid in "${pids[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        alive=1
        break
      fi
    done
    if (( alive == 0 )); then
      break
    fi
    sleep 0.2
  done
  for pid in "${pids[@]}"; do
    kill -KILL "$pid" 2>/dev/null || true
  done
  wait || true
  exit 0
}

trap shutdown TERM INT

start idgen ./idgen -f=../etc2/idgen.yaml
sleep 1

start status ./status -f=../etc2/status.yaml
sleep 1

start authsession ./authsession -f=../etc2/authsession.yaml
sleep 1

start dfs ./dfs -f=../etc2/dfs.yaml
sleep 1

start media ./media -f=../etc2/media.yaml
sleep 1

start biz ./biz -f=../etc2/biz.yaml
sleep 1

start msg ./msg -f=../etc2/msg.yaml
sleep 1

start sync ./sync -f=../etc2/sync.yaml
sleep 1

start bff ./bff -f=../etc2/bff.yaml
sleep 5

start session ./session -f=../etc2/session.yaml
sleep 1

start gnetway ./gnetway -f=../etc2/gnetway.yaml
sleep 1

start linkpreview ./linkpreview -f=../etc2/linkpreview.yaml

wait

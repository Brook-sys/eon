#!/bin/sh
set -eu

image=${1:?usage: container-smoke.sh IMAGE}
expected_revision=${EXPECTED_REVISION:?EXPECTED_REVISION is required}
container=eon-container-smoke
volume=eon-container-smoke-data

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
cleanup
docker volume create "$volume" >/dev/null

start() {
  docker run -d --name "$container" -p 127.0.0.1::8080 -v "$volume:/data" "$image" >/dev/null
}

wait_healthy() {
  i=0
  while [ "$i" -lt 40 ]; do
    status=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$container")
    [ "$status" = healthy ] && return 0
    [ "$status" = unhealthy ] && break
    i=$((i + 1))
    sleep 1
  done
  docker logs "$container"
  return 1
}

start
wait_healthy
port=$(docker port "$container" 8080/tcp | sed 's/.*://')
health=$(curl -fsS "http://127.0.0.1:$port/api/inspect/health")
version=$(curl -fsS "http://127.0.0.1:$port/api/inspect/version")
dashboard=$(curl -fsSL "http://127.0.0.1:$port/")
printf '%s' "$health" | grep -q '"status":"ok"'
printf '%s' "$version" | grep -Fq "$expected_revision"
test -n "$dashboard"
test "$(docker exec "$container" stat -c %u /data/eon.db)" = 10001

docker rm -f "$container" >/dev/null
start
wait_healthy
port=$(docker port "$container" 8080/tcp | sed 's/.*://')
curl -fsS "http://127.0.0.1:$port/api/inspect/health" | grep -q '"status":"ok"'

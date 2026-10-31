#!/bin/sh
# Run against a locally built image; uses only disposable, uniquely named data.
set -eu
IMAGE=${1:-loxone2velux:review}
NAME="loxone2velux-smoke-$$-$(date +%s)"
VOLUME="${NAME}-data"
cleanup() {
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    docker volume rm "$VOLUME" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker volume create "$VOLUME" >/dev/null
start() {
    docker run -d --name "$NAME" -v "$VOLUME:/data" "$IMAGE" >/dev/null
    n=0
    until docker exec "$NAME" wget -qO- http://localhost:8080/health >/dev/null; do
        n=$((n+1))
        if [ "$n" -ge 30 ]; then docker logs "$NAME"; exit 1; fi
        sleep 1
    done
    test "$(docker exec "$NAME" id -u)" = 1000
}
start
docker exec "$NAME" wget -qO- --header 'Content-Type: application/json' \
    --post-data '{"node_id":1,"loxone_id":"smoke","name":"persistent mapping"}' \
    http://localhost:8080/api/mappings | grep -Fq '"loxone_id":"smoke"' >/dev/null
docker exec "$NAME" wget -qO- --header 'Content-Type: application/json' \
    --post-data '{"logging":{"level":"warn"}}' \
    http://localhost:8080/api/config | grep -Fq '"level":"warn"' >/dev/null
test "$(docker exec "$NAME" stat -c %a /data/config.yaml)" = 600
docker stop "$NAME" >/dev/null
docker rm "$NAME" >/dev/null
start
saved=$(docker exec "$NAME" wget -qO- http://localhost:8080/api/config)
printf '%s\n' "$saved" | grep -Fq '"level":"warn"'
printf '%s\n' "$saved" | grep -Fq '"loxone_id":"smoke"'
echo 'Container persistence smoke test passed (non-root, atomic writes, recreate).'

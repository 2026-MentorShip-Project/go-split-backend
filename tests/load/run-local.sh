#!/usr/bin/env bash
# Runs a k6 load test against a local server and a freshly recreated database.
# Usage: tests/load/run-local.sh smoke|capacity
set -euo pipefail

test=${1:?usage: run-local.sh smoke|capacity}
cd "$(dirname "$0")/../.."

command -v k6 >/dev/null || { echo "k6 not found; install it with: brew install k6" >&2; exit 1; }
if curl -fs http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
  echo "port 8080 is already serving; stop the other server first" >&2
  exit 1
fi

db=go_split_load
docker compose up -d --wait db
docker compose exec -T db dropdb -U go_split --if-exists --force "$db"
docker compose exec -T db createdb -U go_split "$db"

export DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=go_split DB_PASSWORD=go_split DB_NAME=$db
export BASE_URL=http://127.0.0.1:8080
export TEST_DATABASE_URL="postgres://go_split:go_split@127.0.0.1:5432/$db?sslmode=disable"

mkdir -p artifacts
go build -o artifacts/load-server ./cmd/go-split-backend
artifacts/load-server >artifacts/load-server.log 2>&1 &
server=$!
trap 'kill "$server" 2>/dev/null || true' EXIT

until curl -fs "$BASE_URL/healthz" >/dev/null 2>&1; do
  kill -0 "$server" 2>/dev/null || { cat artifacts/load-server.log >&2; exit 1; }
  sleep 0.5
done

case $test in
  smoke) RUN_LOAD=1 go test -tags=e2e -count=1 -v ./tests/e2e -run '^TestSmallLoad$' ;;
  capacity) RUN_CAPACITY=1 go test -tags=e2e -count=1 -timeout 30m -v ./tests/e2e -run '^TestCapacity$' ;;
  *) echo "unknown test: $test" >&2; exit 2 ;;
esac

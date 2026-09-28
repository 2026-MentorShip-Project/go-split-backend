#!/usr/bin/env bash
# Measures the latency the frontend's /api proxy adds, at a low request rate.
# Usage: LOAD_BACKEND_URL=https://<cloud-run-url> LOAD_FRONTEND_URL=https://go-split.vercel.app/api \
#        LOAD_SESSION=<cookie> LOAD_EVENT_ID=<id> tests/load/run-hop.sh
set -euo pipefail
cd "$(dirname "$0")/../.."

: "${LOAD_BACKEND_URL:?set LOAD_BACKEND_URL to the backend URL, e.g. the Cloud Run URL}"
: "${LOAD_FRONTEND_URL:?set LOAD_FRONTEND_URL to the frontend proxy, e.g. https://go-split.vercel.app/api}"
: "${LOAD_SESSION:?set LOAD_SESSION to your session cookie value}"
: "${LOAD_EVENT_ID:?set LOAD_EVENT_ID to an event you belong to}"
export LOAD_BACKEND_URL=${LOAD_BACKEND_URL%/} LOAD_FRONTEND_URL=${LOAD_FRONTEND_URL%/}
command -v k6 >/dev/null || { echo "k6 not found; install it with: brew install k6" >&2; exit 1; }

# Also warms both paths, so a cold start is not counted as proxy cost.
for base in "$LOAD_BACKEND_URL" "$LOAD_FRONTEND_URL"; do
  status=$(printf 'header = "Cookie: session=%s"\n' "$LOAD_SESSION" |
    curl -s -o /dev/null -w '%{http_code}' -K - "$base/events/$LOAD_EVENT_ID")
  if [ "$status" != 200 ]; then
    echo "preflight $base/events/$LOAD_EVENT_ID returned $status; check the URL, session and event ID" >&2
    exit 1
  fi
done

mkdir -p artifacts
k6 run -q tests/load/hop.js

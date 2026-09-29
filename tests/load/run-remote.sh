#!/usr/bin/env bash
# Runs the capacity test, reads only, against a deployed server such as production.
# Usage: LOAD_BASE_URL=https://... LOAD_SESSION=<cookie> LOAD_EVENT_ID=<id> tests/load/run-remote.sh
set -euo pipefail
cd "$(dirname "$0")/../.."

: "${LOAD_BASE_URL:?set LOAD_BASE_URL to the backend URL, e.g. the Cloud Run URL}"
: "${LOAD_SESSION:?set LOAD_SESSION to your session cookie value}"
: "${LOAD_EVENT_ID:?set LOAD_EVENT_ID to an event you belong to}"
export LOAD_BASE_URL=${LOAD_BASE_URL%/}
command -v k6 >/dev/null || { echo "k6 not found; install it with: brew install k6" >&2; exit 1; }

# The cookie goes through stdin so it does not show up in the process list.
status=$(printf 'header = "Cookie: session=%s"\n' "$LOAD_SESSION" |
  curl -s -o /dev/null -w '%{http_code}' -K - "$LOAD_BASE_URL/events/$LOAD_EVENT_ID")
if [ "$status" != 200 ]; then
  echo "preflight GET /events/$LOAD_EVENT_ID returned $status; check the URL, session and event ID" >&2
  exit 1
fi

rps=${CAPACITY_RPS:-300}
seconds=$((60 + ${CAPACITY_HOLD_SECONDS:-120} + 15))
echo "About to send reads to $LOAD_BASE_URL, ramping to $rps requests/s, for about ${seconds}s."
echo "No data is written. Real users of that server share the load."
if [ "${CONFIRM:-}" != yes ]; then
  read -r -p "Type yes to continue: " answer
  [ "$answer" = yes ] || { echo "aborted" >&2; exit 1; }
fi

mkdir -p artifacts
k6 run --summary-export artifacts/remote-summary.json tests/load/capacity.js

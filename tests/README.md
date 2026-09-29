# CI tests

`.github/workflows/CI.yaml` runs unit tests, race detection, lint, API E2E,
and a small load test for PRs targeting `main`, including subsequent pushes.
Each integration job uses disposable PostgreSQL 15 and the actual server binary.
The Go test harness provides local HTTPS so secure session cookies work normally.
Only k6's local test certificate verification is disabled.

To run locally, configure `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, and
`DB_NAME` for a disposable PostgreSQL database, then:

```sh
mkdir -p artifacts
go run ./cmd/seed-templates
go run ./cmd/go-split-backend
```

In another terminal:

```sh
export BASE_URL=http://127.0.0.1:8080
go test -tags=e2e -count=1 -v ./tests/e2e -run '^Test(EventJourney|TemplateEventSettings)$'
# Requires k6 v1.6.1 on PATH.
RUN_LOAD=1 go test -tags=e2e -count=1 -v ./tests/e2e -run '^TestSmallLoad$'
```

Load testing sends 20 authenticated event-list requests/second for 30 seconds.
It requires p95 below 500 ms, failed requests below 1%, all response checks
passing, and no dropped iterations. Account/event setup is outside measured
traffic. These are initial CI regression limits, not production capacity claims.
Results and server logs are retained for seven days, including on failure.
Tests create unique accounts/events; discard the test database after use.

## Local load tests

Both targets recreate a `go_split_load` database in the compose Postgres, start
the server on `:8080`, run the test, and stop the server. They need Docker and k6.

```sh
make load-smoke     # the CI load test above
make load-capacity  # ramps to 300 RPS; about 4 minutes
make load-capacity CAPACITY_RPS=150 CAPACITY_HOLD_SECONDS=60
```

The capacity test (`load/capacity.js`, seeded by `TestCapacity`) mixes reads and
saves, adds a busy event and settlements racing saves, and checks p95 per
endpoint. Targets and results are in `docs/report/03-scalability.md`.

To run it against a deployed server such as production:

```sh
make load-prod LOAD_BASE_URL=https://<cloud-run-url> LOAD_SESSION=<cookie> LOAD_EVENT_ID=<id>
```

This mode only reads, because seeding needs database access and settling
freezes an event for good. It runs the everyday-traffic scenario against one
event you belong to, using your own session. Take the `session` cookie from
your browser's developer tools after signing in; the event ID is in the app's
URL. It checks the session and event first, then asks for confirmation
(skip with `CONFIRM=yes`). Real users of that server share the load, and at 300
RPS the database connection limit in the scalability doc applies.

To measure what the frontend's `/api` proxy adds, send the same reads straight to
the backend and through the proxy at a low rate (`HOP_RPS`, default 5, for
`HOP_SECONDS`, default 60), and compare:

```sh
make load-hop LOAD_BACKEND_URL=https://<cloud-run-url> LOAD_FRONTEND_URL=https://go-split.vercel.app/api \
  LOAD_SESSION=<cookie> LOAD_EVENT_ID=<id>
```

Template transaction and database uniqueness checks require a disposable database:

```sh
TEST_DATABASE_URL="postgres://user:password@127.0.0.1:5432/test_db?sslmode=disable" \
  go test -race -tags=integration -count=1 ./internal/events
```

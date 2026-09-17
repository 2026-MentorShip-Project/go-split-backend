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

Template transaction and database uniqueness checks require a disposable database:

```sh
TEST_DATABASE_URL="postgres://user:password@127.0.0.1:5432/test_db?sslmode=disable" \
  go test -race -tags=integration -count=1 ./internal/events
```

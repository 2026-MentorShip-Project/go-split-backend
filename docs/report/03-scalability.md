# 3. Scalability Design

## Expected scale

Go-Split serves small groups: a trip, a barbecue, a dinner. An event has a
handful to a few dozen members and tens to low hundreds of expense lines.
Traffic comes in bursts: members check their share after a host saves, and
everyone looks at once when the host settles. One event rarely sees more than
a few saves per second.

## What the design does for scale

- **Stateless API on Cloud Run.** Sessions live in PostgreSQL, so any instance
  can serve any request. Cloud Run scales from 0 to 10 instances (1 vCPU,
  512 MiB each).
- **Previews run in the browser.** The WASM engine computes splits while the
  host types, so only the final save reaches the server.
- **Per-event write serialization.** Saves, joins and settlement on one event
  run one at a time in a transaction. This keeps results correct, and it
  limits how many writes one event can take per second (see below).
- **Settlement snapshot.** After settling, reads come from a frozen snapshot
  instead of a new calculation.
- **PGO pipeline.** Production CPU profiles go to GCS and can feed
  `go build -pgo` (see `docs/pgo.md`).

## Load testing

Two k6 tests, both run against a local server on a throwaway database
(`go_split_load`, recreated on each run):

| | `make load-smoke` | `make load-capacity` |
| --- | --- | --- |
| Purpose | Regression check in CI | Check the targets below |
| Traffic | `GET /events`, 20 RPS for 30 s | Ramp to 300 RPS over 60 s, hold 120 s |
| Mix | Reads only | 85% reads across five endpoints, 15% saves (a third of them followed by an edit) |
| Extra scenarios | none | 20 saves/s on one busy event; 40 settlements, each racing three saves |
| Runs in CI | Yes | No, because shared runners give noisy timings |

`make load-prod` runs the capacity test's everyday-traffic scenario, reads
only, against a deployed server such as production, using a real session and
event (see `tests/README.md`). It skips saves, the busy event and settlement:
seeding them needs direct database access, and settling freezes events for good.

Targets, from the report spec's reference numbers:

| Metric | Target |
| --- | --- |
| P95 latency, everyday traffic and each endpoint in it | < 500 ms |
| P95 latency, busy event | < 500 ms |
| P95 latency, settlement | < 1000 ms (our own target; settling is rare and does more work) |
| Failed requests and HTTP 5xx | < 1% |
| Throughput | 300 RPS |

Knobs: `CAPACITY_RPS`, `CAPACITY_HOLD_SECONDS`, `CAPACITY_HOT_RPS`, e.g.
`make load-capacity CAPACITY_RPS=150`. The k6 summary is saved to
`artifacts/capacity-summary.json` and the server log to
`artifacts/load-server.log`.

### Results (2026-09-28, local)

Apple M2 with 8 cores, running k6, the server and PostgreSQL 15 (Docker) on the
same machine. About 52,000 requests per run, 0 errors, 0 HTTP 5xx.

| P95 | Default pool (8 connections) | Pool raised to 32 (experiment only) |
| --- | --- | --- |
| Everyday traffic | 385 ms | 148 ms |
| Slowest endpoint in it | 457 ms (`PATCH` item) | 214 ms |
| Busy event | 370 ms | **791 ms** |
| Settlement | 69 ms | 595 ms |
| Dropped iterations | 29 | 0 |

At 300 RPS the API meets every target with the default pool, but with little
room to spare. At 20 RPS the same requests take about 7 ms, so almost all of
the extra latency at 300 RPS is waiting, not work.

The P95 was about the same on every endpoint, including the cheap
`GET /events`. That points to requests queueing for a database connection.
Raising the pool to 32 confirmed it: everyday traffic got 2.6 times faster.
But the busy event got slower, because more connections let more saves reach
that event's lock at once. The pool size change is not committed.

The local numbers are not a production capacity claim. Production has 1 vCPU
per instance, a smaller database, network latency to Cloud SQL, and cold
starts, which the local test does not cover.

## Known limitations and bottlenecks

### Database connections (the main limit)

- **Cloud SQL `db-f1-micro` allows about 25 connections**, a few of them
  reserved for Cloud SQL itself.
- **The API does not set a pool size.** `internal/database/database.go` uses
  the pgx default: max(4, number of CPUs) connections per instance, so at
  least 4.
- **Cloud Run can run 10 instances**, which is at least 40 connections.
  Under a traffic spike the database would refuse connections before the API
  runs out of CPU. The CD migration step, through the Cloud SQL Auth Proxy,
  needs one more.
- **Locally the pool is also the bottleneck**: at 300 RPS, requests wait for
  one of 8 connections (results above).

Options, cheapest first:

1. Set `MaxConns` per instance, from an environment variable, so that
   `MaxConns × max instances` stays under the database limit.
2. Lower Cloud Run's `max_instance_count` to match.
3. Move to a larger Cloud SQL tier, which allows more connections.
4. Put a connection pooler (PgBouncer, or the Cloud SQL managed pooler) in
   front of the database.

### Other limits

- **One event takes writes one at a time.** A busy event stays under 500 ms
  at 20 saves/s only while the pool limits concurrency. That's far above real
  use, but a large, very active event would queue.
- **Shares are calculated when read.** Before settlement, `/shares` and
  `/me/details` recompute every expense line on each request, so their cost
  grows with the number of lines in the event. That's fine at realistic
  sizes; caching becomes worth it only for very large events.
- **Cold starts.** Scaling to zero means the first request after a quiet
  period starts a new instance, which runs migrations and template seeding
  before serving. Not measured.
- **Single-zone database.** `availability_type = "ZONAL"`. A zone outage
  takes the database down until it recovers; backups and point-in-time
  recovery are on.

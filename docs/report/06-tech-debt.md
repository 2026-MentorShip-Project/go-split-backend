# 6. Technical Debt Log

## Performance testing: what we measure and what we don't

Our load tests measure the backend. What a user feels also includes their own
network, the frontend proxy, and the browser rendering the page, and none of
the tests cover that.

### Latency

| What | Status | How, or why not |
| --- | --- | --- |
| Backend time per request, under load | Tested, locally | `make load-capacity`: ramps to 300 RPS; P95 per endpoint (section 3) |
| Backend time per request, production | Can test, reads only | `make load-prod`, straight to Cloud Run |
| Time the Vercel `/api` proxy adds | Can test | `make load-hop` sends the same reads direct and proxied, and reports the difference |
| Production write and settlement latency | Not tested | The test harness logs in by writing sessions to the database, and production has no test login. Settling would permanently freeze real events. |
| Cold start (first request after scale to zero) | Not tested | Needs an idle production service; locally the server is always warm |
| Latency the user feels: tap Save → screen updated | Not tested | Includes the user's network, DNS, TLS, JavaScript and rendering. Needs measurement in the browser (real user monitoring) or scripted browser checks, not a load test |
| Page load: HTML, JavaScript, the WASM engine | Not tested | Frontend concern; served by Vercel |

### Throughput and errors

| What | Status | How, or why not |
| --- | --- | --- |
| 300 RPS with reads and writes | Tested, locally | `make load-capacity`, on one laptop with k6, the server and Postgres sharing it |
| 300 RPS in production | Reads only | `make load-prod`. Real users share the load, and the connection limit below applies |
| Concurrent saves and settlement on one event | Tested, locally | Busy-event and settlement scenarios |
| Multiple Cloud Run instances | Not tested | Local runs have one server process |
| Load through Vercel | Not tested on purpose | Vercel may throttle or block bursts from one IP, and it isn't ours to load test |

**Why it's this way.** Testing the backend directly isolates the part we own
and can tune: Go, the connection pool and Postgres. The proxy only forwards
requests, so it adds latency but not load. Production writes are left out
because there is no safe way to create test data there, and settlement can't
be undone.

**Next step.** Measure real users' timings in the browser, for example
Next.js `useReportWebVitals` or Vercel Speed Insights, plus timings on API
calls.

## Database connections exceed Cloud SQL's limit

- **What.** The pgx pool isn't configured, so each instance opens up to
  max(4, CPU count) connections. Ten Cloud Run instances can open 40 or more,
  but `db-f1-micro` allows about 25.
- **Why.** The pgx defaults were enough for development, and the limit only
  shows under a traffic spike.
- **Impact.** Under a spike, Postgres refuses new connections and requests fail.
  Before that, requests queue for a connection: locally at 300 RPS, P95 was
  385 ms with the default pool and 148 ms with 32 connections (section 3).
- **Fix.** Set `MaxConns` from an environment variable so that
  `MaxConns × max instances` stays under the database limit. Later, a larger
  database tier or a connection pooler.

## Database has backups but no high availability

- **What.** Cloud SQL runs in one zone (`availability_type = "ZONAL"`), with no
  standby and no read replica, and `deletion_protection = false`. Daily backups
  and point-in-time recovery are on. `db-f1-micro` is a shared-core tier, which
  the Cloud SQL uptime SLA doesn't cover.
- **Why.** Lowest cost for a project at this stage. A standby roughly doubles
  the database bill.
- **Impact.** Data can be restored, but a zone outage or instance failure means
  downtime until it recovers. A mistaken delete removes the instance.
- **Fix.** Turn on deletion protection now, since it's free. Switch to
  `REGIONAL` (with a standby in another zone) on a dedicated-core tier when
  uptime matters.

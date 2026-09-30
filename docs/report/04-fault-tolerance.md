# 4. Fault Tolerance & Resilience Design

Go-Split handles money, so the design puts **correctness under failure** first:
a request that fails must leave no half-written data, and a settled event
must never change. Availability comes second: the system depends on one
database in one zone, and a few gaps (listed at the end) are known.

## At a glance

| Failure | What happens | Where |
| --- | --- | --- |
| Handler error or panic mid-request | Transaction rolls back; the client gets an error, never a false success | `internal/events/transaction.go`, `server.go` recovery |
| Two writes to the same event at once | Serialized by a row lock on the event; the second waits | `eventTransaction` (`FOR UPDATE`) |
| Write to a settled event | Rejected by the API (409) and again by database triggers | `transaction.go`, migration `0009` |
| Google sign-in slow or down | 10 s timeout, then 504; other upstream errors are 502 | `internal/auth/google.go` |
| AI rule drafting unavailable | 503; the rest of the app is unaffected | `internal/events/rule_draft.go` |
| Profile upload to GCS fails | Logged and skipped; requests are unaffected | `internal/profiling` |
| Database down at boot | Process exits; Cloud Run restarts the instance | `server.go` (`log.Fatalf`) |
| Instance shut down (deploy, scale-in) | In-flight requests get up to 10 s to finish | `server.go` graceful shutdown |
| Database instance or zone lost | Downtime until recovered; data restorable from backups/PITR | Terraform `deploy/gcp/main.tf` |

## Third-party service failures

The API calls only two outside services at request time, and neither sits on
the path of recording expenses or settling.

**Google tokeninfo (sign-in only).** `POST /auth/google` verifies the ID token
with Google's `tokeninfo` endpoint.

- The HTTP client has a **10 s timeout** and also carries the request context
  (`google.go:63`).
- Failures are mapped to distinct statuses, so the frontend can tell the user
  what went wrong:

  | Cause | Response |
  | --- | --- |
  | `GOOGLE_CLIENT_ID` not configured | 503 `google sign-in not configured` |
  | Token invalid, or Google answers 400 | 401 `invalid google token` |
  | Email not verified by Google | 401 `google_email_unverified` |
  | Timeout | 504 `google_timeout` |
  | Any other upstream error | 502 `verify google token` |

- The response body is read with a 64 KB limit, and the ID token is stripped
  from error messages before they are logged.
- Already signed-in users are unaffected by a Google outage: their session is
  checked against our own database, not Google.

**Vertex AI (rule drafting).** `POST /events/{id}/rules/draft` turns a
plain-language description into draft rules. The Vertex call is not written
yet, so the route answers **503** unless `RULE_DRAFT_FIXTURE=true`, which
serves canned plans for demos and tests. Two choices keep this feature from
hurting the rest of the system once it's live:

- The route sits **outside the event transaction**, so a slow model call never
  holds the event's lock (`rule_draft.go:22-27`).
- A draft is **validated and never saved** (`ruleassist.Check`,
  `refuseLockedCreates`). The host reviews it and saves through the normal,
  validated endpoints.

**Cloud Storage (profiling).** CPU profiles for PGO are uploaded every 10
minutes by a background goroutine. A failure to create the client at startup
is a warning, and a failed upload is logged and skipped. No request depends
on it.

## Timeouts and retries

| Where | Timeout | Retry |
| --- | --- | --- |
| Google tokeninfo | 10 s | None; the user retries sign-in |
| `/healthz` database ping | 1 s | n/a |
| Graceful shutdown | 10 s | n/a |
| Seed/migrate CLI in CD | 2 min | CD waits up to 30 s for the Cloud SQL proxy |
| Invite-code collision on create | n/a | Up to 8 attempts (see gaps) |
| Database queries | None set; pgx defaults | None |
| `http.Server` read/write | None set; Cloud Run's request timeout applies | n/a |
| Frontend `fetch` | None; no `AbortController` | None |

Retries are deliberately rare. Most writes aren't idempotent (see below), so
an automatic retry could record an expense twice. The frontend instead shows
the error and lets the user try again.

## Error handling

**Backend.** Every handler chooses its status explicitly and answers with the
same JSON shape, `{"error": "..."}`:

| Status | Meaning |
| --- | --- |
| 400 | Malformed JSON or path parameter |
| 401 | No valid session |
| 403 | Not a member of the event, or the role lacks permission |
| 404 | Event not found |
| 409 | Conflict: event is read-only, or a rule is in use (with a count and a link to the affected details) |
| 410 | Invite for a settled event |
| 422 | Invalid split, with `details: [{index, code}]` so the frontend can mark each bad line |
| 502 / 503 / 504 | Upstream failure, feature not configured, upstream timeout |
| 500 | Database or unexpected error, with a short fixed message |

A panic is caught by Gin's recovery middleware, logged with its stack, and
answered with 500. The event transaction's deferred rollback still runs, so
a panic can't leave partial writes.

**Frontend.** `readApiError` turns the error shape into a message, appending
any 422/409 codes: `error（code1、code2）`. Pages show errors in an error
banner (settle, rules), a toast (members), or inline under the form (create
event, item detail). The rules page updates optimistically and, if the save
fails, re-fetches from the server so the screen matches the saved state.

## Fallback strategies

- **Rule drafting:** fixture mode (`RULE_DRAFT_FIXTURE`) stands in for Vertex
  in demos and CI; without either, the route fails closed with 503.
- **Split preview:** the browser's WASM engine only validates rules before
  saving. The server recalculates everything on save and at settlement, so a
  broken or outdated WASM engine can't produce a wrong saved result. The
  server's answer is the one that counts.
- **Engine loading:** if the WASM file fails to load, the cached promise is
  cleared so the next call tries again.
- **Sign-in:** if Google is down, guests can still join events by invite
  code, and existing sessions keep working.

## Data consistency

This is the strongest part of the design. Three layers enforce it.

**1. One transaction per event request.** Every `/events/{id}/...` route runs
inside `eventTransaction` (`internal/events/transaction.go`):

1. `BEGIN`, then lock the event row: `FOR UPDATE` for writes, `FOR SHARE` for
   reads. Writes to one event run one at a time; reads don't block each other.
2. Reject writes to a settled or archived event with 409.
3. Run the handler with all its queries on the same transaction.
4. **Buffer the response.** Commit only if the status is below 400. If the
   commit fails, discard the buffered success and answer 500.
5. A deferred rollback covers every error and panic.

So a client never sees a success for data that wasn't saved, and a save and a
settlement can't interleave. Event creation (event, settings, host
membership and invite code) and guest join run in their own single
transactions for the same reason.

**2. The database enforces the rules too.** The API checks aren't the only
guard (migration `0009_prd_alignment.sql`):

- `one_host_per_event`: a unique index allowing exactly one host per event.
- `settlement_state`: a check that settled ⇔ a settlement snapshot exists,
  and archived ⇒ settled.
- `freeze_settlement`: a trigger that stops a settled event from being edited,
  deleted or unarchived.
- `guard_event_child`: triggers on members, items, detail lines, rules and
  tags. Each locks the parent event and rejects the write if it's settled or
  archived. A bug in a handler still can't change a settled event.
- Unique constraints on tag labels per event, one rule per tag, one
  membership per account or guest per event, and detail-line order per item.

**3. Money is exact, and settlement is frozen.**

- Amounts are whole NT dollars stored as `BIGINT` with `CHECK >= 0`. There are
  no floats and no rounding. The engine assigns any remainder by a fixed rule.
  The migration to whole dollars refuses to run if any amount had cents,
  rather than rounding silently.
- `POST /events/{id}/settle` validates every line again, then stores a
  snapshot: inputs, results, member order, host, transfers and **engine
  version**. Later reads come from the snapshot, so an engine upgrade can
  never change a settled result.

**Idempotency.** A second settle gets 409 (the event is already read-only),
and a second join returns the existing membership. Creating an event or an
item is not idempotent. The frontend disables the submit button while a
request is in flight, but there is no server-side idempotency key.

## Startup, deploy and shutdown

- **Boot order:** start the profiler (optional), open the database, run
  migrations, seed templates, then serve. If the database is unreachable the
  process exits, and Cloud Run replaces the instance.
- **Migrations** run once per file in its own transaction, under a Postgres
  advisory lock so two instances booting together don't both apply the same
  file. CD also runs them through the Cloud SQL proxy **before** deploying, so
  a new revision starts against an already-migrated schema.
- **Template seeding** is an upsert, safe to run on every boot.
- **Graceful shutdown:** on `SIGTERM` the server stops accepting connections
  and gives in-flight requests 10 s to finish, then closes the database pool
  and stops the profiler.
- **Scaling:** Cloud Run runs 0 to 5 instances by default
  (`cloud_run_max_instances`). The cap keeps total database connections under
  Cloud SQL's limit (section 3). Scaling to zero means a cold start after
  idle periods.

## Database resilience

Cloud SQL has daily backups and point-in-time recovery, encrypted connections
only, and disk autoresize. It runs in one zone with no standby, so a zone or
instance failure means downtime until it recovers (see section 6).

## Observability and its limits

We rely on what Google Cloud provides by default, with no extra tooling:

| Signal | What we get | Source |
| --- | --- | --- |
| Request logs | One entry per request: method, URL, status, latency, size, user agent, client IP, trace id, instance | Cloud Run, automatic |
| App logs | Whatever the server writes to stdout/stderr (logrus, plain text) | Cloud Run, automatic |
| Metrics | Request count and latency, instance count, CPU, memory, startup latency, concurrency, bytes in/out | Cloud Run, built in |
| Traces | One span per sampled request, at most 1 request per 10 s per instance, free | Cloud Run → Cloud Trace, automatic |
| Profiles | CPU profiles every 10 minutes, for PGO | Our own uploader to GCS |

This shows **that** a request failed, when, and on which instance. It
mostly can't show **why**, or **which endpoint** is slow:

- **Request logs have no content.** They don't include request or response
  bodies or the caller's identity. A failing `POST /events/42/items` is
  visible, but not what was sent or by whom.
- **App logs don't fill the gap yet.** Most 500s don't log their cause. The
  logs are plain text, so Cloud Logging stores them without a severity, and
  without the `logging.googleapis.com/trace` field they aren't grouped under
  their request (section 6).
- **Logs are kept for 30 days.** That's the default for the `_Default` log
  bucket; it can be raised to up to 3,650 days at extra storage cost. A
  problem reported after a month has no logs left.
- **Volume isn't a constraint at our scale.** An entry can be up to 256 KiB,
  so stack traces fit. The ingestion quota is 300 MB per minute per project
  (4.8 GB in the largest regions); past it, writes fail rather than being
  sampled. Reading through the API is limited to 60 list requests per minute,
  so bulk analysis needs a BigQuery export or Log Analytics.
- **Metrics can't be broken down by endpoint.** Request count and latency can
  be grouped by response code and revision, but they have no label for the
  URL path, so "P95 went up" doesn't say which route. A log-based metric could
  add the route, but our paths contain ids (`/events/42/items`), so they'd
  need normalizing first.
- **Metrics are coarse, and detail fades.** They're sampled about once a
  minute, so a spike of a few seconds is averaged away. Data is kept at full
  resolution for 6 weeks, then downsampled to 10-minute intervals or dropped,
  depending on the metric type. Latency percentiles are computed from
  histogram buckets, so they're approximate.
- **Nothing from inside the process.** There's no measure of time spent
  waiting for a pool connection or for the event lock, of query durations,
  or of Go runtime health (goroutines, GC). Section 3 found those waits are
  the main latency factor. Cloud SQL's own metrics cover the database side
  in part.
- **Traces show only the total time.** Cloud Run's automatic trace has one
  span per request, so it can't show which query or lock took the time.
  Child spans need OpenTelemetry instrumentation, which Cloud Trace bills
  for.

The cheapest improvements, in order:

1. JSON logs with `severity`, the trace id, and the error cause on every 500.
2. Log the route pattern (Gin's `c.FullPath()`, e.g. `/events/:id/items`)
   with errors and slow requests.
3. Raise log retention to about 90 days.
4. If needed, OpenTelemetry spans around pool acquisition and queries.

## Known gaps

Recorded in section 6 where they're significant:

- **No health probes.** `/healthz` pings the database, but Terraform
  configures no Cloud Run startup or liveness probe, so it is only used by CI.
- **Errors don't record their cause.** Cloud Run's request logs and metrics
  show which requests failed, but most 500s return a fixed message without
  logging the underlying error, and the app's plain-text logs aren't linked
  to their request by trace id.
- **No query or server timeouts.** A stuck query holds its connection and the
  event lock until Cloud Run's request timeout.
- **Invite-code retry can't recover.** It runs inside the create-event
  transaction without a savepoint. After a collision Postgres aborts the
  transaction, so the next attempt fails and the request returns 500.
  Collisions are rare (31⁶ ≈ 887 million codes), but the retry doesn't do what
  it claims.
- **No server-side idempotency** for creating events and items; a lost
  response plus a user retry creates a duplicate.
- **Settle is two requests from the frontend** (save the note, then settle).
  If the second fails, the note is already saved.
- **The frontend has no 401 handling and swallows some load errors**, so an
  expired session can look like an empty page instead of a sign-in prompt.

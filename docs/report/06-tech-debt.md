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

## Templates are identified by their display label

- **What.** Templates have no stable ID; the Mandarin display label (e.g.
  `烤肉/露營模板`) is the identifier. `allowedTemplates` in
  `internal/events/create.go` is keyed by label and hard-coded rather than read
  from the `templates` table (see the TODO there). Events store the label in
  `events.template`, and `GET /templates/summary?label=` looks templates up by
  label. The label is a query parameter because labels can contain `/`.
- **Why.** The templates are few and written in-house, so labels were unique
  and convenient. Reusing them avoided a schema field and a mapping layer.
- **Impact.** Rewording a label, even to fix a typo, changes the identifier:
  client links break and existing events keep pointing at the old label.
  Translating labels would change the identifier per language. Clients must
  percent-encode labels in URLs, and logs show the encoded form.
- **Fix.** Give each template a stable ASCII key; the embedded JSON filenames
  (`outdoor`, `dinner`, `travel`, `custom`) already work as one. Look templates
  up by key (`/templates/:key/summary`), keep the label for display only, and
  migrate `events.template` from label to key.

## Security gaps

- **What.** Guest identities can be taken over by anyone who knows a
  guest's email and phone. The test-only password routes are live in
  production. The session cookie is `SameSite=None` with no CSRF defence.
  Nothing is rate-limited.
- **Why.** Guests were designed to avoid sign-up friction, and the password
  routes were added "for easy manual testing". The rest isn't stated.
- **Impact and fix.** See section 5, "Open risks".

## Production errors don't record their cause

- **What.** Cloud Run already logs every request (method, URL, status,
  latency, trace id) and records request-count and latency metrics, so the
  platform shows *that* a request failed. The app doesn't say *why*: most 500
  responses return a fixed message without logging the underlying error. The
  app's logs are plain text, so Cloud Logging files them with no severity,
  and without the `logging.googleapis.com/trace` field they aren't linked to
  their request. `/healthz` exists, but Terraform configures no Cloud Run
  probe that uses it.
- **Why.** Not stated. Local debugging was enough during development.
- **Impact.** A production 500 shows up in the request log and the error-rate
  metric, but its cause is lost. App log lines can't be filtered by severity
  or grouped under their request.
- **Fix.** Log the error wherever a 500 is returned. Switch logrus to JSON
  with `severity` and the trace id from `X-Cloud-Trace-Context`. Use
  `/healthz` as the startup probe and a database-free `/livez` as the
  liveness probe.

## CD deploys main without the CI gate

- **What.** CI runs on pull requests only. CD runs on every push to `main`
  without waiting for CI, and applies Terraform with `-auto-approve`, with no
  plan review. CI doesn't check formatting, so unformatted code has reached
  `main` (`internal/events/create.go`).
- **Why.** Not stated. Every change was expected to arrive through a
  reviewed pull request.
- **Impact.** A direct push, or a merge whose CI failed, ships to production,
  and infrastructure changes apply unreviewed.
- **Fix.** Run CI on pushes to `main` and make the deploy job depend on it.
  Add a `gofmt` check, and a `terraform plan` step that needs approval.

## Migrations are forward-only

- **What.** A custom runner applies SQL files in filename order, with no
  down migrations. Two files share the `0004_` prefix. Migrations run in CD
  and again at every instance start. Destructive changes (for example
  `0012_drop_member_binding.sql`) apply while the previous revision is still
  serving.
- **Why.** CD migrates first "so the new revision boots against a
  fully-migrated schema". A migration tool was listed as follow-up work.
- **Impact.** The old revision can fail during a rollout, there's no scripted
  rollback, and every cold start makes extra database round trips.
- **Fix.** Adopt a migration tool (goose or golang-migrate) with an
  expand-then-contract convention, and run migrations in CD only.

## The event lock is taken before the role check

- **What.** `eventTransaction` wraps every `/events/{id}` route. It opens a
  transaction and locks the event row before `RequireEventRole` checks the
  caller's membership (`internal/events/handler.go:31`).
- **Why.** The lock must cover validation and the write, so a save and a
  settlement can't interleave (section 4).
- **Impact.** Any signed-in user can take a lock on any event id. Every
  request holds a pool connection for its whole duration, which makes the
  pool limit above worse. A 404 versus a 403 reveals whether an event id
  exists.
- **Fix.** Check membership before locking, and lock only around writes.

## Sessions are never cleaned up

- **What.** Sessions last 30 days, guests included. Expired rows are never
  deleted; only logout deletes a row. There's no "log out everywhere", and
  signing in again doesn't revoke older sessions.
- **Why.** Not stated.
- **Impact.** The table grows without limit, and a stolen or hijacked session
  can only be revoked by editing the database.
- **Fix.** Periodically delete expired rows, add a revoke-all endpoint, and
  shorten the guest session lifetime.

## Engine changes aren't tied to a version bump

- **What.** The frontend preview runs the same Go engine compiled to WASM,
  pinned at `@go-split/engine` `1.2.0`. The version is checked only when an
  `engine-v*` tag is released. A pull request can change
  `internal/splitengine` or `internal/rulespec` without bumping `Version`
  (`engine.go:11`). The two match today.
- **Why.** The backend stays authoritative, so drift can't corrupt saved
  data.
- **Impact.** After a backend-only engine change, the frontend can accept
  rules the API rejects, or reject rules it accepts, until someone bumps the
  package by hand.
- **Fix.** Fail CI when engine code changes without a `Version` bump, and
  have the frontend compare its engine version with the backend's.

## AI rule drafting is a stub

- **What.** `VertexGenerator.Draft` returns `ErrNotImplemented`, so
  production answers 503. Only `RULE_DRAFT_FIXTURE=true` returns canned
  plans. There are no timeouts, retries, token limits or per-host quotas yet.
- **Why.** Choosing the model and its region is a deployment decision
  (`vertex.go`).
- **Impact.** The endpoint is in the API docs but unusable. Once wired up it
  has no cost limit beyond host-only access and a 500-character input cap.
- **Fix.** Implement the call with a context timeout, one bounded retry, a
  cap on output tokens, and a daily quota per host.

## Database credentials and roles

- **What.** Terraform writes the database password into both the Cloud SQL
  user and Secret Manager, so it's also stored in plain text in the Terraform
  state bucket. The app's database user also runs migrations, so the running
  app can change the schema.
- **Why.** One user and a Terraform-managed secret was the simplest setup.
- **Impact.** Anyone who can read the state bucket has the password, and a
  compromised app could drop tables.
- **Fix.** Create the secret outside Terraform, or use IAM database
  authentication, and give migrations a separate role from the app.

## Frontend: who the user is comes from browser storage

- **What.** There's no `middleware.ts` and no "who am I" request, and nothing
  handles a 401. The app layout decides whether the user is a guest from
  `localStorage.guest_session`, and the join page decides whether they're
  signed in from `sessionStorage.userName`, which is per tab.
- **Why.** The session cookie is `HttpOnly`, and the backend has no
  `GET /auth/me`.
- **Impact.** A signed-in user who opens an invite in a new tab is sent down
  the guest path. An expired session shows empty pages instead of the
  sign-in screen.
- **Fix.** Add `GET /auth/me`, resolve the user from it in Next.js
  middleware, and redirect to sign-in on any 401.

## Frontend: prototype data in the store

- **What.** The zustand store starts with demo data: fake members (小凱 and
  others), a demo account and password, a settled event with a bank-account
  note, and sample items (`src/store/slices/`). Per-event data isn't reset
  when the user switches events. Unused prototype code remains (`src/data/`,
  most of `src/lib/helpers.ts`, several components, and the `useSplitEngine`
  hook).
- **Why.** Left over from the click-through prototype.
- **Impact.** When a load fails, the page shows demo people or the previous
  event's members, rules and tags as if they were real.
- **Fix.** Empty the defaults, reset per-event state when the event id
  changes, and delete the dead code.

## Frontend: errors are swallowed

- **What.** Many loads end in `.catch(() => {})` or an empty `catch {}`, for
  example in the layout, members, rules and create pages. One member-save
  failure always shows "活動結束已無法編輯", whatever the cause.
- **Why.** Not stated.
- **Impact.** Users see blank or out-of-date screens with no message, and
  the 409/422 codes the API sends are lost.
- **Fix.** Add one shared error-and-toast path, and remove the empty
  catches.

## Frontend: API types written by hand

- **What.** Response types in `src/api/*.ts` are written by hand and cast
  from `res.json()` without checking. Recent commits fixed drift, such as
  `invite_code` declared required although the API omits it.
- **Why.** Not stated.
- **Impact.** When the backend changes a response, the frontend still
  compiles and only fails at runtime.
- **Fix.** Generate the types from the backend's Swagger spec (for example
  with openapi-typescript), and check in CI that they're up to date.

## Frontend: no tests or CI

- **What.** There's no test script, no test files and no GitHub workflow.
  Vercel only builds, and lint isn't part of the build.
- **Why.** Not stated.
- **Impact.** Data-loss regressions have shipped and been fixed after the
  fact: item edits dropping details, a date shifting by one day, a join note
  being lost.
- **Fix.** Add a workflow that runs lint, type-checking and the build, unit
  tests for `src/lib`, and one end-to-end smoke test of the create and join
  flows.

## Frontend: invite link hard-codes the production URL

- **What.** The invite link and QR code are built from
  `https://go-split.vercel.app` (`group/page.tsx:72`), not the current
  origin.
- **Why.** Not stated.
- **Impact.** Invites from preview or local deployments send people to
  production.
- **Fix.** Build the link from `window.location.origin`.

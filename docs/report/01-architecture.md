# 1. Application Architecture

Go-Split splits group expenses for an event. A host creates the event, invites
members by code, records expenses, and settles once. The browser previews each
split while the host types; the server calculates the result that counts.

## Diagram

```mermaid
flowchart LR
    subgraph Browser
        UI["Next.js app<br/>React 19 + Zustand"]
        WASM["Split engine (WASM)<br/>live preview"]
        UI --> WASM
    end

    Google["Google Identity<br/>sign-in + tokeninfo"]

    subgraph Vercel
        FE["Next.js server<br/>/api/* rewrite"]
    end

    subgraph GCP["Google Cloud (us-central1)"]
        subgraph Run["Cloud Run: go-split-backend<br/>0–10 instances, 1 vCPU / 512 MiB"]
            API["Gin HTTP API"]
            AUTH["auth<br/>sessions, roles"]
            EVT["events<br/>items, rules, settle"]
            ENG["splitengine + rulespec"]
            PROF["profiling<br/>CPU pprof"]
            API --> AUTH
            API --> EVT
            EVT --> ENG
        end
        SQL[("Cloud SQL<br/>PostgreSQL 15")]
        SM["Secret Manager"]
        GCS[("GCS bucket<br/>PGO profiles")]
        VTX["Vertex AI<br/>(rule assist, not yet routed)"]
        GAR["Artifact Registry"]
    end

    subgraph GitHub
        CI["Actions: CI / CD<br/>npm-engine release"]
        NPM["npm<br/>@go-split/engine"]
    end

    UI -- "HTTPS, same origin" --> FE
    UI -- "ID token" --> Google
    FE -- "HTTPS + session cookie" --> API
    AUTH -- "verify ID token" --> Google
    AUTH --> SQL
    EVT --> SQL
    Run -. "secrets at boot" .-> SM
    PROF -. "upload profiles" .-> GCS
    EVT -. planned .-> VTX

    CI -- "terraform, migrate, image" --> GAR
    GAR --> Run
    CI -- "engine-vX.Y.Z tag" --> NPM
    NPM -- "npm install" --> Vercel
```

## Components

| Component | Where it runs | Job |
| --- | --- | --- |
| Next.js frontend | Vercel (`go-split.vercel.app`) | UI. Rewrites `/api/*` to the backend, so the browser talks to one origin and the session cookie stays first-party. |
| Split engine (WASM) | The user's browser | The same Go split code as the server, compiled to WASM (`cmd/splitengine-wasm`). It previews a split without a network call. Released to npm as `@go-split/engine`; the frontend copies the binary into its build. |
| Go API | Cloud Run, container built `FROM scratch` | Gin router. Runs database migrations and template seeding at startup, then serves HTTP on `:8080`. |
| `internal/auth` | Go API | Google sign-in, guest join by invite code, `HttpOnly` session cookie, and per-event roles (host, co-host, member, guest). |
| `internal/events` | Go API | Events, members, expense items, rules, tags, settlement, archive, and the shares/transfers reads. |
| `internal/splitengine`, `internal/rulespec` | Go API and WASM | Pure calculation: split one amount across members by rules, weights and fixed amounts, then work out who pays the host. No I/O. |
| `internal/profiling` | Go API | Collects CPU profiles in production and uploads them to GCS, used to build Go profile-guided optimization (PGO) (see `docs/pgo.md`). |
| `internal/ruleassist` | Go package only | Drafts split rules from a plain-language description through Vertex AI. Not connected to a route yet. |
| Cloud SQL | GCP | PostgreSQL 15 (`db-f1-micro`, one zone). Backups and point-in-time recovery on. Encrypted connections only; Cloud Run reaches it through the Cloud SQL Unix socket. |
| Secret Manager | GCP | Database password and Google client ID, read by the Cloud Run service account. |

## Main request flows

**Sign in.** The browser gets a Google ID token and sends it to
`POST /auth/google`. The API checks it with Google's `tokeninfo` endpoint (10 s
timeout), creates or finds the account, and sets a session cookie. Guests use
`POST /auth/join` with an invite code instead and get the same kind of cookie.

**Record an expense.** While the host edits, the browser runs the WASM engine
for a preview. On save, the API validates every line, runs the same engine, and
writes the item in one transaction. If any line is invalid, nothing is saved,
and the response is a 422 with a code for each bad line.

**Settle.** The host calls `POST /events/{id}/settle`. The API checks every
line again, then freezes the inputs, results, member order, host, and engine
version into a snapshot. After that the event is read-only; later reads come
from the snapshot, not a new calculation.

## Design decisions

- **One engine, two places.** The preview and the real result come from the
  same Go code, so they can't disagree. The server result is the one that counts.
- **The database enforces the rules too.** Exactly one host per event and
  read-only settled events are database constraints, not only API checks.
  Writes to one event are serialized in transactions, so a save and a
  settlement can't overlap.
- **Whole NT dollars.** Amounts are integers in dollars, with no cents and no
  silent rounding.
- **Same-origin proxy.** Routing API calls through Vercel keeps the cookie
  first-party, so browsers that block third-party cookies still work.
- **Scale to zero.** Cloud Run runs 0 to 10 instances. Idle cost is near zero;
  the trade-off is a cold start on the first request after a quiet period.

## Delivery pipeline

- **CI** (`.github/workflows/CI.yaml`, on PRs to `main`): unit tests with the
  race detector, lint, API end-to-end tests and a k6 load test, each against a
  throwaway PostgreSQL 15.
- **CD** (`cd.yaml`, on push to `main`): Terraform apply (`deploy/gcp`), run
  migrations through the Cloud SQL Auth Proxy, build and push the image to
  Artifact Registry, deploy to Cloud Run.
- **Engine release** (`npm-engine.yaml`, on an `engine-vX.Y.Z` tag): build,
  test and publish `@go-split/engine` to npm. It does not deploy the backend.

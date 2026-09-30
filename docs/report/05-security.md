# 5. Security Design

Go-Split stores who owes whom money, plus members' names, emails and phone
numbers. The risks that matter most are therefore:

- someone **reading or changing an event they don't belong to**;
- a member **doing more than their role allows**, such as editing others'
  expenses or settling;
- a **settled result changing** after the fact;
- **personal data leaking**, especially guests' email and phone numbers.

This section describes how each is handled, then lists the risks that are
still open, most serious first.

## At a glance

| Risk | Status | How |
| --- | --- | --- |
| Reading another event by id (IDOR) | Addressed | Every `/events/{id}` route checks membership; child rows are scoped by `event_id` |
| Exceeding one's role | Addressed | Per-route role checks: host, co-host, member |
| Changing a settled event | Addressed | API returns 409, and database triggers reject the write |
| SQL injection | Addressed | Every query uses pgx placeholders |
| XSS | Mostly addressed | React escapes output; no raw HTML rendering. No CSP |
| Stolen session cookie via script | Addressed | `HttpOnly`, `Secure` cookie; token never in JS or storage |
| Forged Google sign-in | Addressed | Audience, issuer, expiry and verified email checked |
| Secrets in the repo | Addressed | Secret Manager; no keys or `.env` files committed |
| **Guest impersonation** | **Open** | See open risks 1 |
| **Account pre-hijack via password sign-up** | **Open** | See open risks 2 |
| **Cross-site request forgery** | **Open** | See open risks 3 |
| **Brute force and abuse** | **Open** | See open risks 4 |

## Authentication

**Accounts sign in with Google.** The browser gets a Google ID token and
sends it to `POST /auth/google`. The API verifies it with Google's
`tokeninfo` endpoint and checks the claims itself (`internal/auth/google.go`,
`checkClaims`):

- `aud` must be our `GOOGLE_CLIENT_ID`, so a token issued to another app is
  rejected;
- `iss` must be Google;
- `exp` must be in the future;
- `email_verified` must be true;
- `sub` and `email` must be present.

The token is removed from error messages before they're logged. If the
client ID isn't configured, sign-in fails closed with 503.

**Guests join with an invite code.** Members who don't want an account join
through `POST /auth/join` with the event's invite code plus their name, email
and phone. Invite codes are 6 characters from a 31-symbol alphabet with no
look-alike characters, generated with `crypto/rand`. A code stops working
once the event is settled (410).

**Password sign-in exists for testing.** `/auth/register` and `/auth/login`
use bcrypt (default cost 10, 8–72 character passwords) and return one generic
error for a wrong email or password. The code comment says these routes are
"for easy manual testing"; the product flow is Google. They are still
registered in production (open risk 2).

## Sessions

- **Token:** 32 random bytes from `crypto/rand`, base64url-encoded. The server
  always creates a new token at sign-in and never accepts one from the
  client, so session fixation isn't possible.
- **Cookie:** `HttpOnly` (unreadable by JavaScript), `Secure` (HTTPS only),
  `Path=/`, host-only. `SameSite=None` (see open risk 3).
- **Expiry:** 30 days, checked in SQL on every request
  (`expires_at > NOW()`).
- **Logout** deletes the session row and clears the cookie. Deleting an
  account deletes its sessions through a cascading foreign key.
- **Storage:** tokens are stored as plain text in the `sessions` table. A
  database leak would expose live sessions. Storing a hash of the token would
  prevent that.

The frontend never handles the token. Its browser storage only holds
display hints (the guest's event id and role, the user's name).

## Authorization

Two middlewares guard the API (`internal/auth/middleware.go`):

- `RequireSession` returns 401 without a valid session.
- `RequireEventRole` looks up the caller's membership in the event named in
  the path. If they aren't a member, or their role isn't allowed, it returns
  403.

| Who | Can do |
| --- | --- |
| Any member (host, co-host, member) | Read the event, members, items, tags, rules, shares and transfers |
| Co-host | Add expenses; edit or delete **only the expenses they created** |
| Host | Everything above, on any expense; edit the event, members, tags and rules; settle; archive; draft rules |
| Signed-in user | List **their own** events; create events (accounts only; guests get 403); join by code |

Beyond the role check:

- **Child objects are scoped to the event.** Item and member queries filter
  by both the object id and `event_id`, so an id from another event returns
  nothing.
- **References are checked.** The payer, and every member named in a custom
  split, must belong to the event.
- **The host can't be removed or replaced by accident.** The database allows
  exactly one host per event, and it must be an account, not a guest.
- **Settled events are read-only at two layers:** the API returns 409, and
  database triggers reject any write to the event or its members, items,
  rules and tags (section 4).

## API security

**Injection.** Every SQL query uses pgx `$n` placeholders. The only strings
joined into SQL are server-side constants (`FOR UPDATE`, table names from a
fixed list), never user input.

**Input validation.** Request structs have length and range limits:

- event name ≤ 120 and place ≤ 200 characters;
- names and tags ≤ 64 characters;
- amounts are whole dollars from 0 to 10¹²;
- the settlement note is ≤ 2,000 characters, and a rule-draft request ≤ 500.

Detail lines reject unknown JSON fields. A few fields have no limit (item
detail notes, the join note on `/events/join`), and there's no overall
request body size limit.

**CORS.** The backend echoes the `Origin` header only for origins in
`ALLOWED_ORIGINS`, and allows credentials. In production the browser doesn't
use CORS at all: it calls `/api/*` on the Vercel origin, and Vercel forwards
the request to Cloud Run. The production allow-list still includes
`http://localhost:3000`, which it doesn't need.

**Exposure.** Cloud Run accepts traffic from the public internet
(`INGRESS_TRAFFIC_ALL`, `allUsers` invoker), so the backend can be called
directly, bypassing Vercel. This is safe only because every route does its own
session and role checks. The Swagger UI (`/swagger/*`) is also public in
production.

## Frontend

- **XSS:** there's no `dangerouslySetInnerHTML`, `innerHTML` or `eval`, and all
  user content goes through JSX, which escapes it.
- **Security headers:** only `Cross-Origin-Opener-Policy` is set, which the
  Google sign-in popup needs. There's no Content Security Policy,
  `frame-ancestors`/`X-Frame-Options`, or `Referrer-Policy`, so the app can be
  framed by another site.

## Sensitive data and secrets

**Personal data stored:**

- accounts: name, email and Google subject id;
- guests: name, email and phone;
- per event: member display names and notes.

None of it is encrypted at the column level; Cloud SQL encrypts the disk by
default. Logs contain ids and error messages, not tokens or cookies.

**In transit:** browser to Vercel and Vercel to Cloud Run use HTTPS. Cloud Run
reaches Cloud SQL through the Cloud SQL Unix socket, and the instance only
accepts encrypted connections (`ssl_mode = ENCRYPTED_ONLY`).

**Secrets:**

- The database password and Google client ID live in Secret Manager. They're
  injected into Cloud Run as secret references, and the runtime service
  account can read only those two secrets.
- CD authenticates to Google Cloud with Workload Identity Federation, so
  there's no long-lived service account key.
- No `.env`, key or state files are committed; the only committed credentials
  are for local Docker Postgres.
- One weakness: Terraform also writes the database password into its state
  file in the GCS state bucket, so anyone who can read that bucket can read the
  password.

## Infrastructure

- **Container:** built `FROM scratch` with a static Go binary and CA
  certificates only, so there's no shell and almost nothing to exploit.
  There's no `USER` directive, so it runs as root inside the container.
- **Service account:** it has the Cloud SQL client role, access to the two
  secrets, and write access to the profiling bucket only. It also has the
  Vertex AI user role for the rule-draft feature, which isn't built yet.
- **Cloud SQL:** it has a public IP, but no networks are allowlisted, so
  connections go through the IAM-checked Cloud SQL connector. Deletion
  protection is off (section 6).
- **CI:** read-only repository permissions. `golangci-lint` runs, but without
  `gosec`, and there's no dependency or image scanning (no Dependabot,
  `govulncheck` or `npm audit`).

## Domain-specific: money, invite codes, AI drafts

**Money can't be set by the client.** The browser's WASM engine only previews
a split. On every save, and again at settlement, the server recalculates
shares with its own engine and ignores any client-computed totals. Invalid
splits get 422, and nothing is saved. After settlement, results come from a
frozen snapshot that records the engine version, so no later change can alter
them.

**Invite codes are shared secrets.** Every member can see the event's code,
and it travels in links (`?invite=`). It has no expiry or rotation until the
event is settled. A leaked code lets a stranger:

- see the event's name and condition tags (public `GET /auth/invite/{code}`);
- join as a member, which gives read access to every expense and share;
- attempt the guest takeover in open risk 1.

**AI rule drafts can't be abused through prompt injection.** Only the host
can call the rule-draft endpoint, with at most 500 characters. The model's
output is constrained by a response schema, then checked on the server
(`ruleassist.Check`):

- tags and members must exist in the event;
- no duplicate rules;
- conditions must be valid;
- locked rules are refused.

A draft is never saved. The host applies it through the normal, validated
endpoints, so a manipulated draft can at worst suggest bad rules the host
must approve.

## OWASP Top 10 (2021)

| Category | Status |
| --- | --- |
| A01 Broken access control | Membership and role checks on every event route; DB-level freeze. **Gap:** guest impersonation, CSRF |
| A02 Cryptographic failures | TLS everywhere, bcrypt, `crypto/rand` tokens. **Gap:** plaintext session tokens at rest |
| A03 Injection | Parameterized SQL; React escaping |
| A04 Insecure design | Server-authoritative money; frozen settlement. **Gap:** unverified guest identity shared across events |
| A05 Security misconfiguration | **Gaps:** test password routes and Swagger public in production; localhost in CORS list; no CSP |
| A06 Vulnerable components | **Gap:** no dependency or image scanning |
| A07 Identification and authentication failures | Google claims verified; fixation-proof sessions. **Gaps:** no rate limiting; pre-hijack via password sign-up |
| A08 Software and data integrity | WIF in CD; engine version in the snapshot. Actions pinned by tag, not commit SHA |
| A09 Logging and monitoring failures | Cloud Run logs every request and records request metrics. **Gap:** most 500s don't log their cause, and app logs aren't linked to requests (section 4) |
| A10 SSRF | Not applicable: the only outbound calls go to fixed Google URLs |

## Open risks, most serious first

1. **Guests can be impersonated.** A guest identity is global and keyed only
   on (email, phone), and neither is verified. `POST /auth/join` reuses an
   existing guest when the pair matches (`guest.go`, `upsertGuest`). So
   anyone with *any* valid invite code and a victim's email and phone gets a
   session as that guest. That session covers every event the victim belongs
   to, and the join also overwrites the victim's name.
   **Fix:** scope guests to one event, or require proof such as an emailed
   one-time code before reusing an existing guest.
2. **Password sign-up allows account pre-hijack.** Registration doesn't
   verify the email, and Google sign-in links to any existing account with
   the same email (`google.go`, `upsertGoogleAccount`). An attacker who
   registers `victim@gmail.com` first keeps password access after the victim
   signs in with Google.
   **Fix:** disable the password routes in production (register them behind
   an environment flag), and don't auto-link to password-created accounts.
3. **Cross-site request forgery.** The session cookie is `SameSite=None`, and
   there's no CSRF token or `Origin` check. Gin's `ShouldBindJSON` doesn't
   require `Content-Type: application/json`. A malicious page can therefore
   submit a plain form POST that carries the cookie. The easiest targets are
   body-less POSTs such as `/events/{id}/settle`, which can't be undone.
   **Fix:** set `SameSite=Lax`, which works because the browser reaches the
   API through the same-origin `/api` proxy. Also reject writes whose `Origin`
   isn't allowed, or whose content type isn't JSON.
4. **No rate limiting.** Nothing throttles `/auth/join`, `/auth/recover`,
   `/auth/login` or the public invite lookup. Invite codes have about 30 bits
   of entropy (31⁶ ≈ 887 million), and the lookup answers 200/404/410, which
   makes enumeration practical at scale.
   **Fix:** per-IP limits on `/auth/*` (middleware or Cloud Armor), and longer
   invite codes.
5. **Hardening items:**
   - store a hash of the session token;
   - add CSP and `frame-ancestors` headers;
   - serve Swagger only outside production;
   - remove localhost from the production CORS list;
   - add a request body size limit;
   - run the container as non-root;
   - enable `gosec`, Dependabot and `govulncheck`;
   - keep the database password out of Terraform state.

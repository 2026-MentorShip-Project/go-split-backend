# PRD alignment audit and implementation contract

Baseline: f4703b8. Sources: [PRD v0.16](https://github.com/Go-Split/Go-Split/blob/main/PRD/PRD-v0-16.md), [engine v1.1](https://github.com/Go-Split/Go-Split/blob/main/PRD/SPEC-ENGINE-v1-1.md), and [prototype](https://go-split.github.io/Go-Split/).

Accepted decisions: exactly one account-backed host per event; incoming minus outgoing equals the host's net (advanced minus owed). Custom amounts remain fixed and never receive remainder adjustments. The prototype is a UX reference; obsolete prototype behavior is not authoritative.

## Baseline gaps

| ID | Contradiction or gap | Required resolution |
|---|---|---|
| 01 | No settle operation, immutable snapshot, engine version or frozen hub/order | Atomic host-only settlement; historical reads use snapshot |
| 02 | Settled/archived expenses, members and rules remain writable | Serialize mutations with settlement; reject writes after settlement |
| 03 | Multiple hosts, guest promotion and sole-host demotion permitted | Exactly one account-backed host, enforced in API and database |
| 04 | Greedy transfers default; arbitrary hub accepted | Hub-only transfers derived from event host; missing hub is an error |
| 05 | Invalid splits persist (no participant, overflow, mismatch) | Engine validity/diff plus whole-card validation before commit |
| 06 | Custom amounts override rule exclusions | Ignore manual inputs for ruled details in engine; reject them at API |
| 07 | No manual participant subset | Nullable manual_member_ids; empty subset reports no-participant |
| 08 | ID sorting changes remainder recipients | Explicit persistent member order; engine preserves input order |
| 09 | Weight zero becomes one; invalid weights/rule structures accepted | Zero means excluded; input validation 0.1–100, one decimal, AND sets |
| 10 | No traces or separate excluded result; zeros conflate participation | Return explanatory trace, validity, weights and excluded list |
| 11 | No shared browser engine artifact | Expose the same Go engine through a WASM preview adapter |
| 12 | Cents API rounds to a cent | Whole NT-dollar contract, migration without silent rounding |
| 13 | Used tags gain rules; used rules/tags freely deleted | Rule locks and usage-aware deletion; rename references atomically |
| 14 | Template can be replaced after creation | Creation-only seeding; remove apply-template route |
| 15 | Paid flags/endpoints remain after C31 | Remove live payment tracking and API fields |
| 16 | All members see all totals/transfers/pair details | Role/state visibility; non-hosts receive personal results |
| 17 | Pair details omit zero/non-direct lines and do not explain hub net | Personal breakdown walks every expense detail, includes zero rows |
| 18 | Member deletion only considers ownership | Also reject deletion when computed owed amount is nonzero |
| 19 | Password register/login remain | Google-only account authentication; retain existing identity records |
| 20 | Join has optional name, no conditions/note and loose phone validation | Required name for new membership; catalog-only conditions and note |
| 21 | Empty expense cards rejected; PATCH skips nested validation | Permit empty cards; validate every provided detail on both paths |
| 22 | No activity metadata edit endpoint or explicit virtual flag | Host-only metadata edits; expose virtual member identity |
| 23 | Placeholder templates reject creation | Allow listed placeholders with empty settings until content exists |
| 24 | Google errors collapse unverified email and upstream timeout | Stable distinct error codes and retained logs |
| 25 | Tests encode obsolete greedy/ID-order behavior | Replace with acceptance, invariant, authorization and lifecycle tests |
| 26 | Swagger, E2E and load fixtures use obsolete APIs | Regenerate docs and update test setup without production auth bypass |
| 27 | First-time guests cannot read available join conditions before membership | Public invitation-code lookup exposes only event identity and condition catalog |
| 28 | Editing a card replaces all detail IDs | Preserve supplied IDs through edits/reordering; reject foreign/duplicate IDs atomically |

## Scope boundary

This repository is a backend. UI-only requirements (draft forms, dirty checks, persistent red frames, loading states, screen layout, clipboard integration and empty-state copy) belong to the frontend and cannot be implemented here. This change supplies error contracts, atomic operations and a shared preview engine to support them. R2 receipt images, OCR, notifications, payment flags, detailed trace presentation and exports remain out of scope. The engine emits traces now; the UI need not display them in R1.

## Breaking changes and deployment

The implementation commits below replace the obsolete API rather than silently emulate it. Deploy with old server processes stopped, migrate, and update clients together. Money is whole NT dollars. Existing cent amounts must be exactly divisible by 100; migration must stop rather than round fractional dollars. Existing data with invalid host topology or settled events lacking historical snapshots must be repaired explicitly before migration; historical calculations must never be fabricated.

Migration `0009_prd_alignment.sql` runs in one transaction. It refuses legacy settled/archived events, fractional-dollar values (including custom amounts), invalid host topology, and manual amounts on ruled details. Restore a backup if abandoning an upgrade; do not mark the migration applied manually. Legacy closed events need a separately reviewed historical-data migration before this release can deploy to that database. This branch does not fabricate or discard their history.

Active legacy splits are recalculated under engine 1.1.0. Review their results before settlement. Historical password hashes remain in the database to preserve existing identities; password authentication is no longer exposed. Existing payment marks are removed by the migration because C31 removes this feature.

## Source contradictions resolved

| Conflict | Decision applied |
|---|---|
| PRD §10 describes adding a second host and removing the first | User clarification overrides it: one fixed account-backed host, no transfer/promotion |
| Hub inflow/outflow wording conflicts with net = advanced − owed | Incoming − outgoing = host.net; tested for host creditor, debtor and zero-net intermediary |
| Prototype retains greedy/pair routes and payment marks | Engine spec and PRD L10/C31 take precedence; hub-only, no payment tracking |
| Older text rounds cents while R1 says whole NT dollars | Integer dollar API and smallest unit 1; no silent currency conversion at request time |
| Prototype/manual overrides can affect ruled details | PRD L14/L21 takes precedence: engine ignores them; API rejects them |
| PRD defers trace display to R2 while engine spec requires trace results | Emit/persist traces now; frontend presentation can remain R2 |
| PRD leaves snapshot input freezing for engineering assessment | Freeze complete inputs, results, member order, host and engine version |
| PRD says archive has no prerequisites while lifecycle requires settlement | Require settled state only; never require payment confirmation |

## Implementation status and API contract

All 28 backend gaps above are implemented. Frontend-only work in the scope boundary remains outside this repository; the demo was reviewed as a reference, not modified or deployed.

- Accounts: `POST /auth/google` accepts a Google ID token. `/auth/register` and `/auth/login` are removed. Unverified email returns `401 google_email_unverified`; timeout returns `504 google_timeout`.
- Join: `GET /auth/invite/{code}` supplies event name and `cond_tags` before first join; invalid codes return 404 and closed invitations 410. `POST /auth/join` accepts `code`, `email`, numeric `phone`, `name`, optional `cond_tags` and `note`. Existing sessions use `/events/join`. Only the host can edit conditions afterward.
- Metadata: `PATCH /events/{id}` replaces name/place/start/end metadata. Send the complete metadata form; template cannot change after creation. Placeholder templates create empty settings and are marked `soon` in the catalog.
- Expenses: `amount`, `total`, `grand_total`, `owed`, `advanced`, `net` and `custom_amounts` are whole NT dollars. A missing/null amount, fractional amount or legacy `amount_cents` field is rejected. Empty cards are valid. Detail drafts use `tag`; engine input uses `item_tag`.
- Manual splitting: omitted/null `manual_member_ids` uses all members; an empty array is invalid for a detail. `custom_amounts` is a member-ID-to-fixed-amount object. Ruled details must omit manual data. Weight zero excludes; omitted JSON weight defaults to one.
- Editing expenses: PATCH `details` is the complete desired list. Include an existing detail's `id` to retain it; omit `id` for a new line. Omitted old details are deleted. Draft list order becomes ordinal. Invalid or foreign IDs roll back the entire operation.
- Invalid card saves and settlement return 422 with `details: [{item_id?, detail_id?, index, code, diff?}]`. `index` is zero-based. Split codes include `no-participant`, `custom-overflow`, `custom-mismatch`; input codes include `rule-lock`, `invalid-name`, `invalid-amount`, `invalid-participant`, `invalid-custom-amount`, `unknown-item-tag`. Draft IDs need not exist yet. Rule/member changes may invalidate existing lines; settlement scans all lines again.
- Tags: `PATCH /events/{id}/tags/items/{label}` and `/tags/conds/{label}` accept `{ "label": "new name" }`, atomically renaming references. Used rules/tags return 409 on prohibited deletion. Rule deletion conflicts include usage count and a details URL.
- Settlement: host `POST /events/{id}/settle` validates and freezes the snapshot, then returns 204. All later mutations except archive return 409. There is no reopening or recalculation endpoint. Event-level transactions serialize writes, joins and settlement; database constraints also enforce the host and immutable state.
- Reads: host gets full `/shares` and `/transfers`; other members get only personal `/shares` and cannot access full `/transfers` until archive. `/me/details` includes every detail (including zero shares), personal advance/owed/net and personal transfers. Host `/members/{member_id}/breakdown` inspects one member. Expense amounts and rules remain visible to every event member.
- Archive: host `POST /events/{id}/archive` requires settlement, returns 204 and is idempotent. Full read-only shares/transfers become available to all members. `/pairs`, paid-state endpoints, and `/apply-template` are removed.

Swagger is regenerated in `docs/swagger.json`, `docs/swagger.yaml`, and `docs/docs.go`.

## Shared browser engine

The shared implementation is Go compiled to WASM, avoiding a second JavaScript calculation implementation. Build both files with the same Go toolchain:

```sh
GOOS=js GOARCH=wasm go build -o engine.wasm ./cmd/splitengine-wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ./wasm_exec.js
```

Load `wasm_exec.js`, instantiate `engine.wasm` with `new Go().importObject`, and start `go.run(instance)` without awaiting it. The adapter installs `globalThis.goSplitDetail(jsonString)`, returning a JSON string:

```js
const result = JSON.parse(goSplitDetail(JSON.stringify({
  detail: { amount: 101, item_tag: "", custom_amounts: { "3": 20 } },
  members: [{ id: 3, cond_tags: [] }, { id: 2, cond_tags: [] }, { id: 1, cond_tags: [] }],
  rules: [],
  split_order: [3, 2, 1]
})));
// shares: 20, 41, 40; validity: "ok"
```

Use server member order (the `split_order` field, with ID tie-break) for previews. The adapter accepts validated R1 inputs: nonnegative integer dollars, unique member IDs, valid rule weights and catalog references; the API remains authoritative for validation. JavaScript clients must stay within safe integer IDs/amounts. R1 always uses minUnit 1. Integrating this artifact into the separate frontend build is still frontend work.

## Verification

Verified locally against a disposable PostgreSQL instance; no development or production data was modified:

- Unit tests with race detector and coverage; formatting, imports, static analysis and Swagger generation.
- PostgreSQL integration: single-host constraints, settlement snapshot, immutable writes, role visibility, join expiry, rules/tag locks, atomic invalid-save rollback, detail-ID preservation, concurrent settlement/save.
- Migration: exact whole-dollar conversion of both expense and custom amounts; fractional dollars and legacy settled history refuse migration without partial conversion.
- Real HTTP E2E: event journey, template settings, host/co-organizer/member settlement and archive.
- Engine fuzz: over 200,000 cases checking conservation, zero-sum nets and hub balance. Native acceptance cases plus Node execution of the compiled WASM.
- k6: 20 authenticated reads/second for 30 seconds, zero request failures and zero dropped iterations. This is a local smoke test, not a production capacity claim.

Google provider claim/error handling is tested using controlled HTTP responses; the automated tests do not perform a live Google browser login. E2E account/session fixtures require an explicit disposable `TEST_DATABASE_URL`; production has no test authentication bypass.

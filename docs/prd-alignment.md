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

## Scope boundary

This repository is a backend. UI-only requirements (draft forms, dirty checks, persistent red frames, loading states, screen layout, clipboard integration and empty-state copy) belong to the frontend and cannot be implemented here. This change supplies error contracts, atomic operations and a shared preview engine to support them. R2 receipt images, OCR, notifications, payment flags, detailed trace presentation and exports remain out of scope. The engine emits traces now; the UI need not display them in R1.

## Breaking changes and deployment

The implementation commits below replace the obsolete API rather than silently emulate it. Deploy with old server processes stopped, migrate, and update clients together. Money is whole NT dollars. Existing cent amounts must be exactly divisible by 100; migration must stop rather than round fractional dollars. Existing data with invalid host topology or settled events lacking historical snapshots must be repaired explicitly before migration; historical calculations must never be fabricated.

Validation and final API usage are recorded here as implementation completes.

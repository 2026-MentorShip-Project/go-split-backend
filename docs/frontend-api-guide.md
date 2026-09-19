# Frontend API guide

This guide covers the event follow-up changes. The frontend is maintained separately; these backend changes do not alter the demo UI.

## Event metadata

Create with `POST /events` using `name`, optional `place`, and `template`. Update name/place with `PATCH /events/{id}`. Scheduling fields `starts_at` and `ends_at` have been removed from requests, responses, and the live schema. System audit fields such as `created_at` remain. Old immutable settlement JSON retains its original audit metadata.

Migration 0010 drops the old scheduling columns. Deploy with the old application stopped, since the earlier server queries those columns. This migration does not reset accounts, memberships, expenses, or settlements.

## 分帳產出

The [current demo](https://go-split.github.io/Go-Split/) shows four blocks across its 活動 and 分帳 tabs:

| Block | Read endpoint | Response fields |
|---|---|---|
| 全部款項 / 款項現況 | `GET /events/{id}/items` | `items[].details`, each item's `total`; sum item totals or use `/shares.grand_total` |
| 人員分攤結果 | `GET /events/{id}/shares` | `per_member[].member_id`, `owed`, `advanced`, `net` |
| 人員付款流向 | `GET /events/{id}/transfers` | `hub_id`, `transfers[].from_id`, `to_id`, `amount` |
| 留言 | `GET /events/{id}` | `transfer_note` |

Resolve member IDs to display names using `GET /events/{id}/members` (or `GET /events/{id}`'s `members`). Money is whole NT dollars. `net = advanced - owed`: positive means receive, negative means pay.

The host saves 留言 using `PATCH /events/{id}/settlement-note` with `{"note":"Please transfer to the host"}`. An empty string clears the note; omitted/null note is invalid. Maximum 2000 characters. It freezes with the settlement and cannot be edited afterward.

Before settlement these calculations are previews. `POST /events/{id}/settle` validates and freezes the result. After settlement, shares and flows read the stored snapshot. Only the host sees full member totals/flows until archive; ordinary members receive only their own shares and get 403 for the full transfers endpoint. The current demo still contains some obsolete payment-confirmation UI; payment-state APIs remain removed per PRD C31.

## Item allocation versus custom overrides

`GET /events/{id}/items/{item_id}` now includes `details[].allocation`. The existing `custom_amounts` field contains only explicit fixed overrides and is normally `{}` for automatic splits. Do not save computed results into `custom_amounts`, as that would convert automatic shares into fixed inputs.

Example detail response (trace fields shortened):

```json
{
  "id": 42,
  "amount": 101,
  "custom_amounts": {},
  "allocation": {
    "validity": "ok",
    "shares": [
      {"member_id": 7, "amount": 51},
      {"member_id": 8, "amount": 50}
    ],
    "excluded": [],
    "total_weight": 2,
    "unit_price": 50.5
  }
}
```

Host allocations include all shares. Before archive, other members see only their own shares/exclusions. After archive, all members can see the complete allocation. Invalid previews carry their validity/diff rather than pretending they are final. The item-list response does not include this enrichment; use `/shares.per_detail` when rendering allocations across many cards. Submit only editable detail fields when saving; `allocation` is read-only.

## 自訂 template

`自訂` is an embedded, seeded template containing empty `item_tags`, `cond_tags`, and `rules` arrays. Startup and `go run ./cmd/seed-templates` upsert it. `GET /templates` includes it with `soon:false`. Create the event with `"template":"自訂"`.

## 我的付款流向

Call `GET /events/{id}/me/details` after settlement. The response includes:

- `member_id`: the signed-in member.
- `net`: their final advanced-minus-owed total.
- `transfers`: only flows where this member is `from_id` or `to_id`.
- `lines`: every detail's own `owed`, `advanced`, and `net`, including zero amounts.

If `from_id` equals `member_id`, display a payment; if `to_id` equals `member_id`, display a receipt. Resolve the other ID using the member list. An empty array after settlement means no transfers. Before settlement, this personal endpoint returns no transfers because no final flows exist yet; do not label an active event "settled/balanced" based on that empty array.

## Bind a host-created member after joining

The shared event invite link stays unchanged. Never auto-bind by display name.

1. Guest joins with `POST /auth/join` using the common event code. This creates a real membership if necessary.
2. Host loads `GET /events/{id}/members` and explicitly selects the placeholder (`virtual:true`) and the joined person (`virtual:false`).
3. Host calls `POST /events/{id}/members/{placeholder_id}/bind`:

```json
{"joined_member_id": 23}
```

This keeps the placeholder's ID, display, role, conditions, note, and split order. The joined account/guest identity moves onto that member; the duplicate joined membership is removed. Expense payer/author references, manual participant IDs and custom amount keys follow the preserved ID. Existing guest cookies keep working and the preserved member becomes `you:true`; another login is unnecessary. The real person inherits the placeholder's role, so the host should review it before confirming.

The operation changes participant count and recalculates active splits. It is a single event transaction. A member from another event cannot be bound; hosts cannot be merged; already-bound targets return 409. If both records have custom overrides on one detail, 409 asks the host to reconcile them. If merging would leave any invalid allocation, 422 identifies the details and rolls everything back. Binding is disabled after settlement. It needs a host confirmation action in member management, but no additional invitation link or link field.

## Browser join followed by 401

The backend integration test performs `/auth/join` and immediately reads `/events/{id}` using the returned session cookie: 200. The same request without that cookie returns 401. A valid session without event membership returns 403. The path is plural: `/events/{id}`.

For a cross-origin frontend, include credentials on both the join request (to accept the cookie) and subsequent API requests:

```js
const response = await fetch(`${API}/auth/join`, {
  method: "POST",
  credentials: "include",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ code, name, email, phone, cond_tags: [], note: "" })
});
if (!response.ok) throw new Error(`Join failed: ${response.status}`);
const { event_id } = await response.json();
const eventResponse = await fetch(`${API}/events/${event_id}`, {
  credentials: "include"
});
```

For Axios, configure the shared instance with `withCredentials:true`. The backend's `ALLOWED_ORIGINS` must include the exact frontend origin (scheme, hostname and port; no path). Cookies are `HttpOnly; Secure; SameSite=None`, so production requires HTTPS. JavaScript cannot read the session through `document.cookie`; the browser handles it. Browser third-party cookie restrictions can still affect a frontend/API on unrelated sites; a same-site deployment or same-origin API proxy avoids relying on third-party cookies.

Use browser Network/Storage panels to check whether `/auth/join`'s `Set-Cookie` was accepted and whether `/events/{id}` sends `Cookie: session=…`. Do not share the cookie value. See [MDN's credentialed fetch guidance](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch#including_credentials).

A successful backend test does not diagnose the deployed frontend: its actual origin, request options and browser cookie-blocking reason still need inspection.

# Frontend API guide

This guide covers the event follow-up changes. The frontend is maintained separately; these backend changes do not alter the demo UI.

## Event metadata

Create with `POST /events` using `name`, optional `place`, `template`, and optional `starts_at` / `ends_at`. Update name, place and dates with `PATCH /events/{id}`. Scheduling fields are calendar dates in `YYYY-MM-DD`, never timestamps. An omitted `starts_at` on create becomes today in UTC+8; an omitted `ends_at` stays null. On `PATCH`, an absent date keeps the stored one and an empty string clears it, so editing only the name cannot wipe a schedule. The host deletes an active event with `DELETE /events/{id}` (204); settled or archived events return 409. `ends_at` before `starts_at` is refused with 400. System audit fields such as `created_at` remain. Old immutable settlement JSON retains its original audit metadata.

Migration 0010 dropped the original `TIMESTAMPTZ` scheduling columns; migration 0011 restores scheduling as nullable `DATE` columns. Deploy with the old application stopped, since the earlier server queries different columns. Neither migration resets accounts, memberships, expenses, or settlements, and events created while scheduling was absent keep null dates.

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

## QR invite flow

The QR code is rendered by the frontend; the backend does not generate or store a QR image. Encode a frontend URL containing the existing invite code:

```text
https://frontend.example/join?code=4KQ2-8P
```

The frontend `/join` route owns the UI and API selection:

1. Read `code` from the query string.
2. Call `GET /auth/invite/{code}` to validate the invitation and load the event name and available conditions.
3. Render the invite form or account-login option.
4. For a new guest, submit the form to `POST /auth/join` with `code`, `email`, `phone`, `name`, `cond_tags`, and optional `note`.
5. For an already authenticated account or guest session, submit to `POST /events/join` with `code`, plus any name, conditions, and note fields that apply.

The QR payload is only another representation of the invite code; it does not create a second invitation or bypass the existing expiration and settlement checks. Use the frontend's QR component/library to render the URL as SVG or canvas.

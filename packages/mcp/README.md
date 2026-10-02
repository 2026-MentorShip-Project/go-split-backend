# @go-split/mcp

An [MCP](https://modelcontextprotocol.io) server that lets Claude Code and other MCP clients read and edit your Go-Split events. It calls the deployed Go-Split API with your token, so every change shows up for the other members in the web app.

## Setup

1. Sign in at https://go-split.vercel.app.
2. Mint a token. Until the web app has a page for this, run the following in the browser console on that site. The web app proxies `/api/*` to the backend, which is where your session cookie lives.

   ```js
   await (await fetch('/api/auth/tokens', { method: 'POST' })).json()
   ```

   The response is `{ "token": "...", "expires_at": "..." }`. Treat the token like a password. It can do anything your session can, including deleting your account.
3. Add the server to Claude Code:

   ```sh
   claude mcp add go-split -e GO_SPLIT_TOKEN=<token> -- npx -y @go-split/mcp
   ```

| Variable | Required | Default |
|---|---|---|
| `GO_SPLIT_TOKEN` | yes | |
| `GO_SPLIT_API_URL` | no | `https://go-backend-api-605450358080.asia-east1.run.app` |

A token lasts 30 days, like a browser session. When it expires, every tool returns a 401 with a hint, and you repeat step 2. To revoke a token early, call `POST /auth/logout` with `Authorization: Bearer <token>`.

## Tools

| Tool | Does | Needs |
|---|---|---|
| `list_events` | Lists your events, with your role and whether each is settled or archived | any member |
| `get_balances` | Members and their ids, each member's owed, advanced and net, and the transfers that settle the event | any member; only the host sees everyone and the transfers before archive |
| `add_expense` | Adds one expense card paid by one member, with one or more lines. A line is split by the event's tag rules, or among `member_ids` if given | host or co |
| `settle_up` | Permanently freezes the event and stores the final transfers. Marked destructive, so clients ask before calling it | host |

Money is whole NT dollars. A positive `net` means the member receives money; a negative `net` means they pay.

## Development

```sh
npm install
npm test
```

The tests connect a real MCP client to the server in memory and replace `fetch`, so they need no backend.

## Releasing

Bump `version` in `package.json`, merge, then push a matching tag:

```sh
git tag mcp-v0.1.0 <reviewed-commit>
git push origin mcp-v0.1.0
```

`.github/workflows/npm-mcp.yaml` tests and packs the package, then publishes it unless that version already exists. It uses the same npm setup as `@go-split/engine`; see `docs/npm-engine-release.md`.

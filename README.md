# go-split-backend



Table of contents
=================
* [PRD dest](#prd)
* [Prerequisite](#prerequisite)
* [Development](#development)
    * [Install modules](#install-modules)
    * [Run pre-commit check](#run-pre-commit-check)
    * [Test on dev cluster](#test-on-dev-cluster)


## Prd
- PRD: https://github.com/Go-Split/Go-Split/blob/main/PRD/SPEC-ENGINE-v1-1.md
- SplitEngine PRD: https://github.com/Go-Split/Go-Split/blob/main/PRD/PRD-v0-16.md
- Demo site: https://go-split.github.io/Go-Split/

## Prerequisite

* [go](https://formulae.brew.sh/formula/go)
* [gin-swagger](https://github.com/swaggo/gin-swagger)
* [golangci-lint](https://golangci-lint.run/)
* [gomock](https://github.com/uber-go/mock)
* [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports)

```shell
make install-tool
```

## Development

### Install modules

```shell
# install modules
go mod download
```

### Configuration

The server reads `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, and `DB_NAME`
for Postgres. Set `GOOGLE_CLIENT_ID` to the OAuth client ID your frontend uses
for Google Sign-In to enable `POST /auth/google`; when it is unset that
endpoint answers 503.


### Run locally

Bring up Postgres in Docker, then run the server (or launch it via the
VSCode "Debug server" configuration for breakpoints):

```shell
docker compose up -d db
DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=go_split DB_PASSWORD=go_split \
    DB_NAME=go_split go run ./cmd/go-split-backend
```

Migrations and template seeds run on startup, so a fresh compose volume is
usable immediately.

### Run pre-commit check

```shell
# run unit test & linter
make pre-commit-check
```
### Account identity schema

Registered identities are stored in `accounts`. The `events`, `event_members`,
and `sessions` tables reference them through `account_id`; `host` remains an
event membership role. Migration `0008_accounts.sql` renames the existing table,
columns, constraints, indexes, and ID sequence while preserving data. The server
applies pending migrations at startup. Deploy this version with old server
instances stopped, because earlier versions query the previous names.

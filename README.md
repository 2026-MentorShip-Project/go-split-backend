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
- link to prd: https://github.com/Go-Split/Go-Split/blob/main/%E7%BE%A4%E9%AB%94%E6%B4%BB%E5%8B%95%E5%88%86%E5%B8%B3%E5%B7%A5%E5%85%B7_PRD_v0.3.md

* [example](https://go-split.github.io/Go-Split/)

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

### Run pre-commit check

```shell
# run unit test & linter
make pre-commit-check
```
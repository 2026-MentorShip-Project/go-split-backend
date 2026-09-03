# go-split-backend

Table of contents
=================
* [Prerequisite](#prerequisite)
* [Development](#development)
    * [Install modules](#install-modules)
    * [Run pre-commit check](#run-pre-commit-check)
    * [Test on dev cluster](#test-on-dev-cluster)


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

### Run pre-commit check

```shell
# run unit test & linter
make pre-commit-check
```
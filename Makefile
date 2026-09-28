REQ_EXECUTABLES := helm kubectl vault consul-template kubectx

PROJECT_ID := project-4ddffd8b-3b42-486b-b6a
REGION := asia-east1
DB_NAME := go-split-postgres

GOOGLE_CLIENT_ID ?=

.PHONY: install-tool
install-tool:
	go install go.uber.org/mock/mockgen@v0.6.0
	go install github.com/swaggo/swag/cmd/swag@v1.16.4
	go install golang.org/x/tools/cmd/goimports@v0.41.0
	brew install golangci-lint


#############  Testing  #############
.PHONY: generate
generate:
	go generate ./...


.PHONY: test
test:
	go test -v -cover -race ./...


####################### Pre-Commit Check ##################

.PHONY: fmt
fmt:
	@echo "==> Tidying imports and simplifying format..."
	@go mod tidy
	@goimports -w .
	@gofmt -s -w .


.PHONY: fmt-check
fmt-check:
	@echo "==> Checking if files are formatted and imports are tidied..."
	@if [ -n "$$(gofmt -s -l .)" ] || [ -n "$$(goimports -l .)" ]; then \
		echo "Format check failed! Run 'make fmt' to fix the following files:"; \
		gofmt -s -l .; \
		goimports -l .; \
		exit 1; \
	fi
	@echo "Format check passed"


.PHONY: lint-check
lint-check:
	@echo "==> Running static analysis..."
	@golangci-lint run ./...


.PHONY: pre-commit-check
pre-commit-check: fmt-check lint-check generate test
	@echo "Success! All checks (Fmt/Lint/Test) passed"


####################### DB ##################
# needs to download cloud-sql-proxy first
# check https://docs.cloud.google.com/sql/docs/mysql/sql-proxy for more details
.PHONY: connect-cloud-sql
connect-cloud-sql:
	@echo "==> Connecting to Cloud SQL..."
	./cloud-sql-proxy --port 5432 $(PROJECT_ID):$(REGION):$(DB_NAME)

.PHONY: swag
swag:
	@echo "==> Regenerating swagger docs..."
	@cd cmd/go-split-backend && swag init -d ../../ -g cmd/go-split-backend/server.go -o ../../docs


####################### Local ##################
.PHONY: serve-login
serve-login:
	@echo "==> Open http://localhost:3000/login.html"
	python3 -m http.server 3000 --directory tests/e2e

.PHONY: run-local
run-local:
	@echo "==> Running the local server..."
	docker compose up -d db
	DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=go_split DB_PASSWORD=go_split \
	GOOGLE_CLIENT_ID=$(GOOGLE_CLIENT_ID) \
	DB_NAME=go_split go run ./cmd/go-split-backend/server.go

####################### Load ##################
# Both start a local server on a throwaway database (go_split_load) and need k6.
# Capacity knobs: CAPACITY_RPS (300), CAPACITY_HOLD_SECONDS (120), CAPACITY_HOT_RPS (20).
.PHONY: load-smoke
load-smoke:
	@./tests/load/run-local.sh smoke

.PHONY: load-capacity
load-capacity:
	@./tests/load/run-local.sh capacity

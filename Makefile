
DOCKER_DEV_REPO := asia-docker.pkg.dev/appier-docker/docker-ai-rec-asia/go-split-backend-dev
DOCKER_TAG := $(DEV_NAME) 

CHART_DIR := ./deploy/go-split-backend
RELEASE_NAME := go-split-backend-dev-$(DEV_NAME)

DEV_CLUSTER := gke_appier-k8s-ai-rec_asia-east1_nelson
DEV_NAMESPACE := rec

REQ_EXECUTABLES := helm kubectl vault consul-template kubectx


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
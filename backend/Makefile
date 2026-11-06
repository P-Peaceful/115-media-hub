GOLANGCI_VERSION := v2.15.0
GOIMPORTS_VERSION := v0.43.0

.PHONY: dev generate format format-check lint vet test test-race build check

dev:
	go run ./cmd/mediahub

generate:
	go generate ./internal/http/generated

format:
	go run golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION) -local github.com/wenzhengcheng/115-media-hub/backend -w $$(find . -name '*.go' -not -name '*.gen.go')

format-check:
	@output="$$(gofmt -l $$(find . -name '*.go' -not -name '*.gen.go'))" && test -z "$$output" || { echo 'gofmt differences found'; exit 1; }
	@output="$$(go run golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION) -local github.com/wenzhengcheng/115-media-hub/backend -l $$(find . -name '*.go' -not -name '*.gen.go'))" && test -z "$$output" || { echo 'goimports differences found'; exit 1; }

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run ./...

vet:
	go vet ./...

test:
	CGO_ENABLED=1 go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

build:
	mkdir -p ../bin
	CGO_ENABLED=1 go build -o ../bin/mediahub-dev ./cmd/mediahub

check: format-check vet test test-race lint

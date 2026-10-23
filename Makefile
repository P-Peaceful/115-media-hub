SHELL := sh
.SHELLFLAGS := -eu -c
.DEFAULT_GOAL := help

.PHONY: help install dev-backend dev-frontend generate check test build smoke docker-build
help:
	@echo 'install | dev-backend | dev-frontend | generate | check | test | build | smoke | docker-build'

install:
	cd backend && go mod download
	pnpm --dir frontend install --frozen-lockfile

dev-backend:
	cd backend && CGO_ENABLED=1 go run -buildvcs=false ./cmd/server

dev-frontend:
	pnpm --dir frontend dev

generate:
	cd backend && go tool oapi-codegen -config ../api/oapi-codegen.yaml ../api/openapi.yaml
	pnpm --dir frontend generate:api

check:
	@test -z "$$(gofmt -l $$(find backend -name '*.go'))" || { echo 'Run gofmt on Go sources'; exit 1; }
	cd backend && go vet ./...
	pnpm --dir frontend typecheck
	pnpm --dir frontend lint
	pnpm --dir frontend peers check
	$(MAKE) test

test:
	cd backend && CGO_ENABLED=1 go test ./...
	pnpm --dir frontend test

build:
	pnpm --dir frontend build
	node scripts/prepare-webui.mjs
	cd backend && CGO_ENABLED=1 go build -buildvcs=false -tags embedui -trimpath -o bin/115-media-hub ./cmd/server

smoke: build
	python3 scripts/smoke.py

docker-build:
	docker build -f deploy/Dockerfile -t 115-media-hub:local .

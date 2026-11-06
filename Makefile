.PHONY: install dev-frontend dev-backend generate check-generated check check-frontend check-backend test-e2e test-integration build smoke docker-build docker-smoke

install:
	cd frontend && pnpm install --frozen-lockfile
	cd backend && go mod download

dev-frontend:
	cd frontend && pnpm dev

dev-backend:
	cd backend && go run ./cmd/mediahub

generate:
	cd backend && go generate ./...
	cd frontend && pnpm generate:api

check-generated:
	python3 scripts/check-generated.py

check: check-frontend check-backend check-generated

check-frontend:
	cd frontend && pnpm format:check && pnpm lint && pnpm typecheck && pnpm test:run && pnpm build

check-backend:
	cd backend && make check

test-e2e:
	cd frontend && pnpm test:e2e

test-integration:
	python3 scripts/test-integration.py

build:
	bash scripts/build.sh

smoke:
	python3 scripts/smoke.py

docker-build:
	docker build -t 115-media-hub:local .

docker-smoke:
	python3 scripts/smoke.py --docker 115-media-hub:local

#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
pnpm --dir frontend build
# The production embed package contains only real Vite output.
mkdir -p backend/internal/web/dist bin
find backend/internal/web/dist -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
cp -R frontend/dist/. backend/internal/web/dist/
(cd backend && CGO_ENABLED=1 go build -trimpath -ldflags "-X github.com/wenzhengcheng/115-media-hub/backend/internal/http.Version=${MEDIAHUB_VERSION:-dev}" -tags production -o ../bin/mediahub ./cmd/mediahub)

# syntax=docker/dockerfile:1
FROM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
RUN npm install --global pnpm@11.28.5
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

FROM golang:1.26-bookworm AS backend
WORKDIR /src/backend
ENV CGO_ENABLED=1
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/frontend/dist/ ./internal/webui/dist/
RUN go build -tags embedui -trimpath -ldflags="-s -w" -o /out/115-media-hub ./cmd/server

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install --yes --no-install-recommends ca-certificates curl tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 mediahub && useradd --uid 10001 --gid mediahub --no-create-home mediahub \
    && mkdir -p /data /storage/strm /storage/quarantine /cache /logs /backups \
    && chown -R mediahub:mediahub /data /storage /cache /logs /backups
COPY --from=backend /out/115-media-hub /usr/local/bin/115-media-hub
WORKDIR /data
ENV MEDIA_HUB_HTTP_ADDR=0.0.0.0:8080 MEDIA_HUB_DATABASE_PATH=/data/115-media-hub.db MEDIA_HUB_LOG_DIR=/logs TZ=Asia/Shanghai
USER mediahub
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD curl --fail --silent http://127.0.0.1:8080/health/ready || exit 1
ENTRYPOINT ["/usr/local/bin/115-media-hub"]

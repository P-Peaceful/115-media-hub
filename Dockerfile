FROM node:24.13.0-bookworm-slim AS frontend
WORKDIR /source/frontend
RUN npm install --global pnpm@11.28.5
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
COPY api/ /source/api/
RUN pnpm build

FROM golang:1.26.9-bookworm AS backend
WORKDIR /source/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /source/frontend/dist/ ./internal/web/dist/
RUN CGO_ENABLED=1 go build -trimpath -tags production -o /out/mediahub ./cmd/mediahub

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 mediahub \
    && useradd --uid 10001 --gid mediahub --no-create-home mediahub \
    && mkdir --parents /app/data /app/logs /app/media /app/cache /app/backups /app/storage/quarantine \
    && chown --recursive mediahub:mediahub /app/data /app/logs /app/media /app/cache /app/backups /app/storage
WORKDIR /app
COPY --from=backend /out/mediahub /app/mediahub
USER 10001:10001
ENV MEDIAHUB_HTTP_ADDR=:8080 \
    MEDIAHUB_DATABASE_PATH=/app/data/mediahub.db \
    MEDIAHUB_LOG_FILE=/app/logs/mediahub.log
EXPOSE 8080
VOLUME ["/app/data", "/app/logs", "/app/media", "/app/cache", "/app/storage/quarantine", "/app/backups"]
ENTRYPOINT ["/app/mediahub"]

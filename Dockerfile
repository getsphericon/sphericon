# Production image: builds the frontend, then a self-contained Go binary with
# the SPA and tracker embedded, then a minimal runtime. Used for `docker build`.
# Releases (GoReleaser) use Dockerfile.release instead,
# which just wraps the prebuilt binary.

# --- Stage 1: frontend (SPA + tracker bundle) ---
FROM node:26-alpine AS frontend
RUN npm install -g pnpm@11
WORKDIR /src
COPY . .
RUN pnpm install --frozen-lockfile
RUN pnpm build:tracker && pnpm build

# --- Stage 2: Go binary with embedded assets ---
FROM golang:1.27-alpine AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed targets: tracker snippet + built SPA from the frontend stage.
COPY --from=frontend /src/packages/analytics/dist/t.js internal/server/assets/t.js
COPY --from=frontend /src/dist/ internal/server/assets/spa/
ARG VERSION=docker
ARG COMMIT=none
RUN go build -tags embed_spa \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /sphericon ./cmd/server

# --- Stage 3: runtime ---
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 sphericon
COPY --from=gobuild /sphericon /usr/local/bin/sphericon
# Run unprivileged from an empty directory, never as root in /.
WORKDIR /app
USER sphericon
# Containers are production deployments: enforce the strict secret checks by default.
ENV APP_ENV=production
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- "http://localhost:${PORT:-3000}/healthz" || exit 1
ENTRYPOINT ["sphericon"]

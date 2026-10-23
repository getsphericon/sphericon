# Production image: builds the frontend, then a self-contained Go binary with
# the SPA and tracker embedded, then a minimal runtime. Used for `docker build`.
# Releases (GoReleaser) use Dockerfile.release instead,
# which just wraps the prebuilt binary.

# --- Stage 1: frontend (SPA + tracker bundle) ---
FROM node:26.12.1-alpine3.24@sha256:40cd011b55415bc894e45b02fdd5a5dfde056921b7d23a64c74f80497af48797 AS frontend

RUN npm install -g pnpm@12.10.1
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY packages/analytics/package.json packages/analytics/
COPY packages/emaildoc/package.json packages/emaildoc/

RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build:tracker && pnpm build

# --- Stage 2: Go binary with embedded assets ---
FROM golang:1.27.3-alpine3.24@sha256:d530cf60723abf1ffae0cd16bb546b379ec0550d2f3815e5b16ed69dbc00fba4 AS gobuild

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed targets: tracker snippet + built SPA from the frontend stage.
COPY --from=frontend /src/packages/analytics/dist/t.js internal/server/assets/t.js
COPY --from=frontend /src/dist/ internal/server/assets/spa/
ARG VERSION=docker
ARG COMMIT=
ARG DATE=
RUN cp CHANGELOG.md internal/changelog/CHANGELOG.md \
    && go build -tags embed_spa,embed_changelog \
    -ldflags "-s -w -X github.com/getsphericon/sphericon/internal/buildinfo.Version=${VERSION} -X github.com/getsphericon/sphericon/internal/buildinfo.Commit=${COMMIT} -X github.com/getsphericon/sphericon/internal/buildinfo.Date=${DATE}" \
    -o /sphericon ./cmd/server

# --- Stage 3: runtime ---
FROM alpine:3.24@sha256:71e9b7bae160ba167fa17414401b94cbcdaf3c32feeab9298bd794a2a0614c35
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 sphericon
COPY --from=gobuild /sphericon /usr/local/bin/sphericon
# Run unprivileged from an empty directory, never as root in /.
WORKDIR /app
USER sphericon
# Containers are production deployments: enforce the strict secret checks by default.
ENV SPHERICON_ENV=production
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- "http://localhost:${PORT:-3000}/healthz" || exit 1
ENTRYPOINT ["sphericon"]

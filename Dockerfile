# Production image: builds the frontend, then a self-contained Go binary with
# the SPA and tracker embedded, then a minimal runtime. Used for `docker build`.
# Releases (GoReleaser) use Dockerfile.release instead,
# which just wraps the prebuilt binary.

# --- Stage 1: frontend (SPA + tracker bundle) ---
FROM node:26.13.1-alpine3.24@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS frontend

RUN npm install -g pnpm@12.21.0
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY packages/analytics/package.json packages/analytics/
COPY packages/emaildoc/package.json packages/emaildoc/
# pnpm-workspace.yaml patches dependencies (patchedDependencies): the install reads the patches.
COPY patches/ patches/

RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build:tracker && pnpm build

# --- Stage 2: Go binary with embedded assets ---
FROM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS gobuild

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
RUN cp CHANGELOG.md internal/domain/instance/upgradenote/CHANGELOG.md \
    && CGO_ENABLED=0 go build -tags embed_spa,embed_changelog \
    -ldflags "-s -w -X github.com/getsphericon/sphericon/internal/kernel/buildinfo.Version=${VERSION} -X github.com/getsphericon/sphericon/internal/kernel/buildinfo.Commit=${COMMIT} -X github.com/getsphericon/sphericon/internal/kernel/buildinfo.Date=${DATE}" \
    -o /sphericon ./cmd/server

# --- Stage 3: the runtime files scratch lacks ---
FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS rootfs
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /rootfs/etc/ssl/certs /rootfs/usr/share \
    && cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ \
    && cp -R /usr/share/zoneinfo /rootfs/usr/share/ \
    && mkdir -m 1777 /rootfs/tmp

# --- Stage 4: runtime ---
# scratch: no shell and no package manager, only the static binary (CGO_ENABLED=0), the CA roots
# for outbound TLS, the zoneinfo a TZ setting reads and an empty /tmp.
FROM scratch
COPY --from=rootfs /rootfs/ /
COPY --from=gobuild /sphericon /usr/local/bin/sphericon
# Run unprivileged from an empty directory, never as root in /.
WORKDIR /app
USER 10001:10001
# Containers are production deployments: enforce the strict secret checks by default.
ENV SPHERICON_ENV=production
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["sphericon", "healthcheck"]
ENTRYPOINT ["sphericon"]

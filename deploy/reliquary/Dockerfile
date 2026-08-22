# Reliquary — the standalone save-sync service.

# ---- frontend build ----
# Two frontends: reliquary's own page, and the companion's — this image
# cross-builds the companion exe it hands out, and that exe embeds its
# page the same way this service embeds its own.
FROM docker.io/library/node:24-alpine AS frontend
WORKDIR /app/reliquary
COPY web/reliquary/package.json web/reliquary/package-lock.json* ./
RUN npm install
COPY web/reliquary/ ./
RUN npm run build
WORKDIR /app/companion
COPY web/companion/package.json web/companion/package-lock.json* ./
RUN npm install
COPY web/companion/ ./
RUN npm run build

# ---- backend build ----
FROM docker.io/library/golang:1.26-alpine AS backend
WORKDIR /app
# Download modules against the committed go.mod/go.sum before copying
# sources, so source-only changes reuse the cached module layer and the
# build can't drift from the committed dependency versions.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/reliquary/dist ./web/reliquary/dist
COPY --from=frontend /app/companion/dist ./web/companion/dist
# The build this image came from, stamped into both binaries and shown
# in both UIs — so a save-sync report can name which service talked to
# which companion. A plain `go build` leaves it "dev".
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-X main.version=${VERSION}" -o /out/reliquary ./cmd/reliquary
# The Artificer Companion, cross-compiled for the machines players
# actually run (docs/companion.md). Shipped inside the image so the
# service hands it out itself — each player's token-gated download link.
# GitHub releases carry the same exe for everyone else.
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -ldflags="-H windowsgui -X main.version=${VERSION}" -o /out/artificer-companion.exe ./cmd/companion

# ---- runtime ----
FROM docker.io/library/alpine:3.22
# Overridable so deployments can match the host owner of the /data bind
# mount (e.g. TrueNAS's apps user, 568:568) at build time; at runtime a
# compose `user:` line does the same job without rebuilding.
ARG UID=1000
ARG GID=1000
RUN addgroup -g "$GID" reliquary && adduser -D -u "$UID" -G reliquary reliquary
WORKDIR /app
COPY --from=backend /out/reliquary ./reliquary
COPY --from=backend /out/artificer-companion.exe ./artificer-companion.exe
RUN mkdir -p /data && chown reliquary:reliquary /data
USER reliquary
# The app otherwise defaults to ./data, which this non-root user can't
# create — the same trap every console image documents.
ENV DATA_DIR=/data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["./reliquary"]

# ---- frontend build ----
FROM docker.io/library/node:24-alpine AS frontend
WORKDIR /app/web
COPY web/wildskeeper/package.json web/wildskeeper/package-lock.json* ./
RUN npm install
COPY web/wildskeeper/ ./
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
COPY --from=frontend /app/web/dist ./web/wildskeeper/dist
RUN CGO_ENABLED=0 go build -o /out/wildskeeper ./cmd/wildskeeper
# The player-side companion, cross-compiled for the machines players
# actually run (games/dragonwilds/docs/companion.md). Shipped inside the
# console image so the console can hand it out itself — the Adventurers
# page serves it behind the companion token.
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -ldflags="-H windowsgui" -o /out/wkcompanion.exe ./cmd/wkcompanion

# ---- runtime ----
FROM docker.io/library/alpine:3.22
# No python3: the plan expected the save reader to shell out to a Python
# GVAS tool, but the format turned out to be SPUD and the reader (dwsave)
# is pure Go inside the binary.
# Overridable so deployments can match the host owner of the /data bind
# mount (e.g. TrueNAS's apps user, 568:568) at build time; at runtime a
# compose `user:` line does the same job without rebuilding — the binary
# only needs a writable DATA_DIR, nothing tied to this account.
ARG UID=1000
ARG GID=1000
RUN addgroup -g "$GID" wildskeeper && adduser -D -u "$UID" -G wildskeeper wildskeeper
WORKDIR /app
COPY --from=backend /out/wildskeeper ./wildskeeper
COPY --from=backend /out/wkcompanion.exe ./wkcompanion.exe
RUN mkdir -p /data && chown wildskeeper:wildskeeper /data
USER wildskeeper
# The app otherwise defaults to ./data, which this non-root user can't
# create — so an image run without DATA_DIR set died with a confusing
# "permission denied" despite /data existing and being owned correctly.
ENV DATA_DIR=/data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["./wildskeeper"]

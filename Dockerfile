# ---- frontend build ----
FROM node:24-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# ---- backend build ----
FROM golang:1.26-alpine AS backend
WORKDIR /app
# Download modules against the committed go.mod/go.sum before copying
# sources, so source-only changes reuse the cached module layer and the
# build can't drift from the committed dependency versions.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /out/dwcon ./cmd/dwcon

# ---- runtime ----
FROM alpine:3.22
# python3 stays for a future Dragonwilds save reader (the Phase 3 gate in
# docs/dragonwilds-recon.md expects to shell out to a Python GVAS tool);
# no save-parsing packages are installed until that reader exists.
RUN apk add --no-cache python3
RUN adduser -D -u 1000 dwcon
WORKDIR /app
COPY --from=backend /out/dwcon ./dwcon
RUN mkdir -p /data && chown dwcon:dwcon /data
USER dwcon
# The app otherwise defaults to ./data, which this non-root user can't
# create — so an image run without DATA_DIR set died with a confusing
# "permission denied" despite /data existing and being owned correctly.
ENV DATA_DIR=/data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["./dwcon"]

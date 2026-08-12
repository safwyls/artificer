# Ilmari — the host provisioning service. One per machine.
FROM docker.io/library/golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /out/ilmari ./cmd/ilmari

FROM docker.io/library/alpine:3.21
COPY --from=build /out/ilmari /usr/local/bin/ilmari
# Runs as root by default because it must read the docker socket, which is
# root-owned on most hosts. Where the socket is group-readable, run as that
# group instead — this service holds the most dangerous handle on the
# machine and should have the least privilege that still works.
EXPOSE 8820
# The image ships no curl; the binary probes itself.
HEALTHCHECK --interval=30s --timeout=5s CMD ["/usr/local/bin/ilmari", "-healthz"]
ENTRYPOINT ["/usr/local/bin/ilmari"]

# Multi-stage build for claude-proxy. Produces a small statically-linked
# binary running as non-root. Used by infra/docker-compose.gate.yml.

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/claude-proxy \
    ./cmd/claude-proxy

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates && \
    addgroup -S proxy && adduser -S -G proxy -u 10001 proxy
COPY --from=build /out/claude-proxy /usr/local/bin/claude-proxy
USER proxy
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/claude-proxy"]

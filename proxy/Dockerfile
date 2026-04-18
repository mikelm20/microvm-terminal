# Multi-stage build for claude-proxy. Produces a ~15 MB statically-linked
# binary running as non-root on scratch. Used by infra/docker-compose.gate.yml
# and, later, by bootstrap.sh for the prod host (`go install` path).

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
    addgroup -S learn && adduser -S -G learn -u 10001 learn
COPY --from=build /out/claude-proxy /usr/local/bin/claude-proxy
USER learn
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/claude-proxy"]

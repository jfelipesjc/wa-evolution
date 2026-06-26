# syntax=docker/dockerfile:1

# ---- build stage ----
# Pure-Go build (modernc.org/sqlite => no CGO), so the binary is fully static
# and the runtime image stays tiny.
FROM golang:1.22-alpine AS build
WORKDIR /src
# Cache module downloads independently of source changes.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wa-server ./cmd/wa-server

# ---- runtime stage ----
# ca-certificates: outbound HTTPS (WhatsApp media, Chatwoot/webhooks).
# tzdata: correct timestamps. Runs as an unprivileged user.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 wa \
 && mkdir -p /data/instances && chown -R wa:wa /data
COPY --from=build /out/wa-server /usr/local/bin/wa-server
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
USER wa
# WA_APIKEY is empty by default which DISABLES auth — set it in production.
ENV WA_ADDR=":8080" \
    WA_DIR="/data/instances" \
    WA_APIKEY=""
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

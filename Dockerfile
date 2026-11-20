# Stage 1: Build pure Go single-binary with embedded Web UI (zero external deps)
FROM golang:1.23-alpine AS go-build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /noc-server ./cmd/noc-server

# Stage 2: Minimal Alpine runtime (~15 MB Image, ~10 MB RAM)
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Europe/Berlin
COPY --from=go-build /noc-server /usr/local/bin/noc-server
EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD wget -q --spider http://127.0.0.1/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/noc-server"]

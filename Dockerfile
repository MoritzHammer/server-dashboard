# Stage 1: Build static Hugo frontend
FROM alpine:3.20 AS hugo-build
RUN apk add --no-cache hugo
WORKDIR /src
COPY hugo.toml ./
COPY content ./content
COPY data ./data
COPY layouts ./layouts
COPY static ./static
RUN hugo --minify && chmod -R a+rX /src/public

# Stage 2: Build zero-dependency Go live NOC collector & HTTP server
FROM golang:1.23-alpine AS go-build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /noc-server ./cmd/noc-server

# Stage 3: Minimal Alpine runtime (~18 MB total)
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Europe/Berlin
WORKDIR /srv
COPY --from=hugo-build /src/public ./public
COPY --from=go-build /noc-server /usr/local/bin/noc-server
EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD wget -q --spider http://127.0.0.1/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/noc-server"]

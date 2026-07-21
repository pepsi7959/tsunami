# syntax=docker/dockerfile:1
#
# Isolated, standalone Tsunami worker image.
# Build context must be the repo root (needs go.mod / the whole module):
#   docker build -f worker/Dockerfile -t tsunami-worker:standalone .

# ---- builder ----
FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tsunami ./worker

# ---- runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/tsunami /usr/local/bin/tsunami
COPY worker/config.standalone.yaml /etc/tsunami/config.yaml

# 8090 admin HTTP control API, 8091 per-test metrics
EXPOSE 8090 8091
ENTRYPOINT ["tsunami", "--path", "/etc/tsunami", "--file", "config.yaml"]

# syntax=docker/dockerfile:1

# ---- builder ----
FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ocean   ./master
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tsunami ./worker

# ---- ocean (master) runtime ----
FROM alpine:3.20 AS ocean
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/ocean /usr/local/bin/ocean
COPY deploy/ocean.yaml /etc/tsunami/config.yaml
EXPOSE 8080
ENTRYPOINT ["ocean", "--path", "/etc/tsunami", "--file", "config.yaml"]

# ---- tsunami (worker) runtime ----
FROM alpine:3.20 AS tsunami
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/tsunami /usr/local/bin/tsunami
COPY deploy/tsunami.yaml /etc/tsunami/config.yaml
EXPOSE 8050 8090 8091
ENTRYPOINT ["tsunami", "--path", "/etc/tsunami", "--file", "config.yaml"]

# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Build stage
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /relay ./cmd/relay

# Runtime stage — alpine (not scratch) for wget health checks
FROM alpine:3
COPY --from=build /relay /relay
EXPOSE 8080
ENTRYPOINT ["/relay"]

# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# golang 1.26-alpine
FROM golang@sha256:e0174e51e81218523251d85d248a90d24c3d5e81543b4f07a5d66229397db190 AS build
ARG BUILD_TAGS
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath ${BUILD_TAGS:+-tags "$BUILD_TAGS"} -o /relay ./cmd/relay

# distroless static-debian12
FROM gcr.io/distroless/static-debian12@sha256:d75cdd72874d4790092fcb1b058493ecf6bb5bf2b2b897045b00ff01d91843f2
COPY --from=build /relay /relay
EXPOSE 8080
ENTRYPOINT ["/relay"]

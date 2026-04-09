# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# golang 1.26-alpine
FROM golang@sha256:ec4debba7b371fb2eaa6169a72fc61ad93b9be6a9ae9da2a010cb81a760d36e7 AS build
ARG BUILD_TAGS
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath ${BUILD_TAGS:+-tags "$BUILD_TAGS"} -o /relay ./cmd/relay

# distroless static-debian12
FROM gcr.io/distroless/static-debian12@sha256:20bc6c0bc4d625a22a8fde3e55f6515709b32055ef8fb9cfbddaa06d1760f838
COPY --from=build /relay /relay
EXPOSE 8080
ENTRYPOINT ["/relay"]

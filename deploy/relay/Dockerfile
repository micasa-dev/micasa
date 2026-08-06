# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# golang 1.26-alpine
FROM golang@sha256:2005724102f45917a63e9d092fc0e4ea56ea575048ce147caad5f5f61502c365 AS build
ARG BUILD_TAGS
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath ${BUILD_TAGS:+-tags "$BUILD_TAGS"} -o /relay ./cmd/relay

# distroless static-debian12
FROM gcr.io/distroless/static-debian12@sha256:a9fcaedd4c9b59e12dd65d954f0b5044f19b0647a8a3712e77205df9e7b102cd
COPY --from=build /relay /relay
EXPOSE 8080
ENTRYPOINT ["/relay"]

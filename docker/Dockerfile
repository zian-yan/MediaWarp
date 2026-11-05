# syntax=docker/dockerfile:1

FROM golang:1.26 AS builder

ARG APP_VERSION=dev
ARG COMMIT_HASH=unknown
ARG BUILD_DATE=unknown

ENV GO111MODULE=on \
    GOTOOLCHAIN=auto \
    CGO_ENABLED=0

WORKDIR /builder
COPY . .
RUN go mod download && \
    go build -ldflags " \
    -s -w \
    -X github.com/AkimioJR/MediaWarp/internal/config.appVersion=${APP_VERSION} \
    -X github.com/AkimioJR/MediaWarp/internal/config.commitHash=${COMMIT_HASH} \
    -X github.com/AkimioJR/MediaWarp/internal/config.buildDate=${BUILD_DATE}" \
    -o MediaWarp

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

ENV GOMEMLIMIT=192MiB

COPY --chmod=755 --from=builder /builder/MediaWarp /MediaWarp

EXPOSE 9000
VOLUME ["/etc/localtime", "/etc/timezone", "/config", "/logs"]
ENTRYPOINT ["/MediaWarp"]

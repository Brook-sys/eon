# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
    -ldflags="-s -w -X main.buildVersion=${VERSION}+${REVISION}" \
    -o /out/eon ./cmd/runtime

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 eon \
    && adduser -S -D -H -u 10001 -G eon eon \
    && install -d -o eon -g eon /data
COPY --from=build /out/eon /usr/local/bin/eon

ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$REVISION

USER eon:eon
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -T 2 -O /dev/null http://127.0.0.1:8080/api/inspect/health || exit 1
ENTRYPOINT ["/usr/local/bin/eon"]
CMD ["-store=sqlite", "-sqlite-path=/data/eon.db", "-listen=0.0.0.0:8080"]

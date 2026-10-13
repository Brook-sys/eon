# syntax=docker/dockerfile:1
#
# Casca EON (D08/D47/D54) — imagem IMUTÁVEL, tags shell-vN no GHCR.
# O binário vivo melhora LOCALMENTE via artefatos vN (D11/D41/ADR-0004):
# esta imagem é só a casca: supervisor + binário eon inicial.
#
# Rebuilda SOMENTE quando supervisor/Dockerfile/CI mudam + gatilho manual
# (D47, workflow shell-image.yml). Tags nunca são regravadas (D54).

FROM golang:1.25-alpine AS build

# Go já pinado por minor no go.mod (D42); builder segue o minor do módulo.
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
WORKDIR /src

# O módulo tem ZERO dependências externas (D19): não há `go mod download`.
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY tools ./tools

ARG SHELL_VERSION=dev
RUN go build -ldflags="-s -w -X main.version=${SHELL_VERSION}" -o /out/eon ./cmd/eon \
    # Smoke: binário precisa responder version (estático por construção, CGO off).
    && /out/eon version

FROM alpine:3.23
# git: operações de repo do runtime (clone/push, D11); ca-certificates: HTTPS
# para o adapter de modelo (M2). Nada mais entra na casca.
RUN apk add --no-cache git ca-certificates

COPY --from=build /out/eon /usr/local/bin/eon

ENV EON_DATA=/data
VOLUME ["/data"]

# Healthcheck nativo (D53): o supervisor publica heartbeat em arquivo de
# estado; `eon health` lê e responde. Zero superfície de rede.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD ["eon", "health"]

# PID 1 = supervisor (D07): docker + supervisor + sidecar autoheal = as
# camadas externas que garantem "nunca para" na casca.
ENTRYPOINT ["eon"]
CMD ["supervisor"]

# syntax=docker/dockerfile:1
# Build multi-stage del api-gateway. Contexto de build: raíz del repositorio.

ARG GO_VERSION=1.22

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
# Caché de módulos y de compilación compartida entre builds (BuildKit): go build
# baja solo lo que compila (no las dependencias de test) y no se repite por servicio.

COPY pkg ./pkg
COPY gateway ./gateway
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd gateway && go build -ldflags="-s -w" -o /out/api-gateway ./cmd/gateway

FROM alpine:3.20
RUN adduser -D -H -u 10001 app
USER app
COPY --from=build /out/api-gateway /app/api-gateway
EXPOSE 8080
ENTRYPOINT ["/app/api-gateway"]

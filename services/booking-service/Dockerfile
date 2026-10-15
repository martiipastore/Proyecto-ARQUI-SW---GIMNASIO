# syntax=docker/dockerfile:1
# Build multi-stage de booking-service. Contexto de build: raíz del repositorio.
# La misma imagen contiene los dos binarios: booking-service (api) y booking-indexer.

ARG GO_VERSION=1.22

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
# Caché de módulos y de compilación compartida entre builds (BuildKit): go build
# baja solo lo que compila (no las dependencias de test) y no se repite por servicio.

COPY pkg ./pkg
COPY services/booking-service ./services/booking-service
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/booking-service \
    && go build -ldflags="-s -w" -o /out/booking-service ./cmd/api \
    && go build -ldflags="-s -w" -o /out/booking-indexer ./cmd/indexer

FROM alpine:3.20
RUN adduser -D -H -u 10001 app
USER app
COPY --from=build /out/booking-service /app/booking-service
COPY --from=build /out/booking-indexer /app/booking-indexer
EXPOSE 8082 8086
# Por defecto la api; el contenedor booking-indexer usa entrypoint /app/booking-indexer.
ENTRYPOINT ["/app/booking-service"]

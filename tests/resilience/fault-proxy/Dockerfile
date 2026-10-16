FROM golang:1.22-alpine AS build
WORKDIR /src
COPY main.go ./main.go
RUN CGO_ENABLED=0 go build -trimpath -o /fault-proxy main.go

FROM alpine:3.20
RUN adduser -D -H -u 10001 app
USER app
COPY --from=build /fault-proxy /app/fault-proxy
ENTRYPOINT ["/app/fault-proxy"]

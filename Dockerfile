# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/callfxo .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
  && addgroup -S app \
  && adduser -S -G app app

WORKDIR /app
COPY --from=builder /out/callfxo /app/callfxo
COPY web /app/web
COPY config.yaml.example /app/config.yaml.example

USER app

EXPOSE 8080/tcp
EXPOSE 5060/tcp
EXPOSE 5060/udp
EXPOSE 12000-12100/udp

ENTRYPOINT ["/app/callfxo"]

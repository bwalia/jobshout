FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy host CA certs for corporate proxy/VPN TLS interception
COPY certs/ /usr/local/share/ca-certificates/
RUN update-ca-certificates || true

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /jobshout-server ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /jobshout-server .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./jobshout-server"]

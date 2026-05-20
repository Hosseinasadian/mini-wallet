FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY . .
RUN go mod tidy && go mod vendor

RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=vendor \
    -o wallet \
    ./cmd/wallet/main.go


FROM alpine:3.21

WORKDIR /app

COPY deployment/wallet/stage/config.yaml /app/deployment/wallet/stage/config.yaml
COPY internal/wallet/repository/migrations /app/internal/wallet/repository/migrations
COPY --from=builder /app/wallet .

CMD ["./wallet", "serve", "--migrate-up"]

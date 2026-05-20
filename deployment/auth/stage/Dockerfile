FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY . .
RUN go mod tidy && go mod vendor

RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=vendor \
    -o auth \
    ./cmd/auth/main.go


FROM alpine:3.21

WORKDIR /app

COPY deployment/auth/stage/config.yaml /app/deployment/auth/stage/config.yaml
COPY internal/auth/repository/migrations /app/internal/auth/repository/migrations
COPY --from=builder /app/auth .

CMD ["./auth", "serve", "--migrate-up"]
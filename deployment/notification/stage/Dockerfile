FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY . .
RUN go mod tidy && go mod vendor


RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=vendor \
    -o notification \
    ./cmd/notification/main.go


FROM alpine:3.21

WORKDIR /app

COPY deployment/notification/stage/config.yaml /app/deployment/notification/stage/config.yaml
COPY --from=builder /app/notification .

CMD ["./notification", "serve"]

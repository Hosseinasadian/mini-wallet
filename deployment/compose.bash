#!/bin/bash

SERVICE="${1:?Usage: compose.bash <service> <env> [docker compose args...]}"
ENV="${2:?Usage: compose.bash <service> <env> [docker compose args...]}"
shift 2

PROJECT_DIR="$(pwd)"
SERVICE_DIR="$PROJECT_DIR/deployment/$SERVICE"
ENV_FILE="$SERVICE_DIR/$ENV/.env"
ENV_EXAMPLE="$SERVICE_DIR/$ENV/.env.example"
PROJECT_NAME="mini_wallet-$ENV"

if [ ! -f "$ENV_FILE" ]; then
    if [ -f "$ENV_EXAMPLE" ]; then
        echo "📝 Creating .env from .env.example..."
        cp "$ENV_EXAMPLE" "$ENV_FILE"
        echo "✅ Created $ENV_FILE — please edit with actual values before running!"
    fi
fi

COMPOSE_FILES=()
for f in "$SERVICE_DIR/$ENV/docker-compose.yaml" "$SERVICE_DIR/$ENV"/*/docker-compose.yaml; do
    [ -f "$f" ] && COMPOSE_FILES+=(-f "$f")
done

docker compose \
    --project-directory "$PROJECT_DIR" \
    --project-name "$PROJECT_NAME" \
    "${COMPOSE_FILES[@]}" \
    "$@"
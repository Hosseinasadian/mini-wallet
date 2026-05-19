#! /bin/bash

docker compose \
--project-directory . \
--project-name mini_wallet-stage \
-f ./deployment/infrastructure/stage/mariadb/docker-compose.yaml \
-f ./deployment/infrastructure/stage/rabbit/docker-compose.yaml \
-f ./deployment/infrastructure/stage/redis/docker-compose.yaml \
-f ./deployment/infrastructure/stage/traefik/docker-compose.yaml \
"$@"
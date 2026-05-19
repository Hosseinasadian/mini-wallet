#! /bin/bash

docker compose \
--project-directory . \
--project-name mini_wallet-stage \
-f ./deployment/notification/stage/docker-compose.yaml \
"$@"
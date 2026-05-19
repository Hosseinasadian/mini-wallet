#! /bin/bash

docker compose \
--project-directory . \
--project-name mini_wallet-stage \
-f ./deployment/auth/stage/docker-compose.yaml \
"$@"
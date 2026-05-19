#! /bin/bash

docker compose \
--project-directory . \
--project-name mini_wallet-stage \
-f ./deployment/docs/stage/docker-compose.yaml \
"$@"
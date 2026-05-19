#! /bin/bash

docker compose \
--project-directory . \
--project-name mini_wallet-stage \
-f ./deployment/wallet/stage/docker-compose.yaml \
"$@"
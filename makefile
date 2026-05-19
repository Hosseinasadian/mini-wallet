# Deployment script paths
ROOT_DIR := $(shell pwd)

DOCS_DIR        := $(ROOT_DIR)/internal/docs/services
DEPLOYMENT_DIR  := $(ROOT_DIR)/deployment
INFRASTRUCTURE_DIR := $(ROOT_DIR)/deployment/infrastructure

check-network:
	@docker network inspect docker_wallet-network > /dev/null 2>&1 || docker network create docker_wallet-network

# ================================
# Service Management - Stage
# ================================

start-auth-stage: ## Start auth service in stage
	$(DEPLOYMENT_DIR)/auth/docker-compose-stage.bash --profile auth --profile stage up -d --build

stop-auth-stage: ## Stop auth service in stage
	$(DEPLOYMENT_DIR)/auth/docker-compose-stage.bash --profile auth --profile stage down

start-wallet-stage: ## Start wallet service in stage
	$(DEPLOYMENT_DIR)/wallet/docker-compose-stage.bash --profile wallet --profile stage up -d --build

stop-wallet-stage: ## Stop wallet service in stage
	$(DEPLOYMENT_DIR)/wallet/docker-compose-stage.bash --profile wallet --profile stage down

start-notification-stage: ## Start notification service in stage
	$(DEPLOYMENT_DIR)/notification/docker-compose-stage.bash --profile notification --profile stage up -d --build

stop-notification-stage: ## Stop notification service in stage
	$(DEPLOYMENT_DIR)/notification/docker-compose-stage.bash --profile notification --profile stage down

# ================================
# Docs Management
# ================================

start-docs-stage: ## Start docs service in stage
	$(DEPLOYMENT_DIR)/docs/docker-compose-stage.bash --profile docs --profile stage up -d --build

stop-docs-stage: ## Stop docs service in stage
	$(DEPLOYMENT_DIR)/docs/docker-compose-stage.bash --profile docs --profile stage down

# ================================
# Infrastructure Tools
# ================================

start-infrastructure-stage: ## Start staging infrastructure tools
	$(INFRASTRUCTURE_DIR)/docker-compose-stage.bash --profile infrastructure --profile stage up -d

stop-infrastructure-stage: ## Stop staging infrastructure tools
	$(INFRASTRUCTURE_DIR)/docker-compose-stage.bash --profile infrastructure --profile stage down

# ================================
# Orchestration
# ================================

start-services-stage: ## Start all services in staging
	@echo "Starting application services..."
	@$(MAKE) start-auth-stage
	@$(MAKE) start-wallet-stage
	@$(MAKE) start-notification-stage
	@echo "All services started successfully!"

stop-services-stage: ## Stop all services in staging
	@echo "Stopping application services..."
	@$(MAKE) stop-notification-stage
	@$(MAKE) stop-wallet-stage
	@$(MAKE) stop-auth-stage
	@echo "All services stopped successfully!"

# ================================
# Swagger Documentation
# ================================

swagger-generate-auth:
	cd internal/auth/delivery/http && swag init -g server.go -o $(DOCS_DIR)/auth --pd --parseInternal --ot json

swagger-generate-wallet:
	cd internal/wallet/delivery/http && swag init -g server.go -o $(DOCS_DIR)/wallet --pd --parseInternal --ot json

swagger-generate-notification:
	cd internal/notification/delivery/http && swag init -g server.go -o $(DOCS_DIR)/notification --pd --parseInternal --ot json

swagger-generate:
	@echo "Generating swagger documentation..."
	@$(MAKE) swagger-generate-auth
	@$(MAKE) swagger-generate-wallet
	@$(MAKE) swagger-generate-notification
	@echo "Swagger documentation generated successfully!"
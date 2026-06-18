# Deployment script paths
ROOT_DIR := $(shell pwd)

ENV_FILE := $(ROOT_DIR)/.env
EXAMPLE_FILE := $(ROOT_DIR)/.env.example
DOCS_DIR        := $(ROOT_DIR)/internal/docs/services
DEPLOYMENT_DIR  := $(ROOT_DIR)/deployment
PROTO_DIR := $(ROOT_DIR)/proto
GEN_DIR := $(ROOT_DIR)/gen/go
NETWORK_NAME := docker_wallet-network

COMPOSE := $(DEPLOYMENT_DIR)/compose.bash

define start-build-service
	$(COMPOSE) $(1) $(2) --profile $(1) --profile $(2) up -d --build
endef

define start-service
	$(COMPOSE) $(1) $(2) --profile $(1) --profile $(2) up -d
endef

define stop-service
	$(COMPOSE) $(1) $(2) --profile $(1) --profile $(2) down
endef

define proto-generate
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(GEN_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=paths=source_relative \
		$$(find $(PROTO_DIR)/$(1)/v1 -name "*.proto")
endef

ensure-env:
	@if [ ! -f "$(ENV_FILE)" ]; then \
		if [ -f "$(EXAMPLE_FILE)" ]; then \
			echo "Copying $(EXAMPLE_FILE) to $(ENV_FILE)..."; \
			cp $(EXAMPLE_FILE) $(ENV_FILE); \
		else \
			echo "Error: $(EXAMPLE_FILE) not found!"; \
			exit 1; \
		fi; \
	fi

check-network:
	@docker network inspect $(NETWORK_NAME) > /dev/null 2>&1 || docker network create $(NETWORK_NAME)

# ================================
# Service Management - Stage
# ================================

start-auth-stage: ## Start auth service in stage
	$(call start-build-service,auth,stage)

stop-auth-stage: ## Stop auth service in stage
	$(call stop-service,auth,stage)

start-wallet-stage: ## Start wallet service in stage
	$(call start-build-service,wallet,stage)

stop-wallet-stage: ## Stop wallet service in stage
	$(call stop-service,wallet,stage)

start-notification-stage: ## Start notification service in stage
	$(call start-build-service,notification,stage)

stop-notification-stage: ## Stop notification service in stage
	$(call stop-service,notification,stage)

# ================================
# Docs Management
# ================================

start-docs-stage: ## Start docs service in stage
	$(call start-build-service,docs,stage)

stop-docs-stage: ## Stop docs service in stage
	$(call stop-service,docs,stage)

# ================================
# Infrastructure Tools
# ================================

start-infrastructure-stage: ## Start staging infrastructure tools
	@$(MAKE) check-network
	$(call start-service,infrastructure,stage)

stop-infrastructure-stage: ## Stop staging infrastructure tools
	$(call stop-service,infrastructure,stage)

# ================================
# Infrastructure Tools
# ================================
proto-generate-common:
	$(call proto-generate,common)

proto-generate-auth:
	$(call proto-generate,auth)

proto-generate-notification:
	$(call proto-generate,notification)


# ================================
# Orchestration
# ================================

start-services-stage: ## Start all services in staging
	@echo "Starting application services..."
	@$(MAKE) check-network
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
	@echo "Generating swagger documentation...."
	@$(MAKE) swagger-generate-auth
	@$(MAKE) swagger-generate-wallet
	@$(MAKE) swagger-generate-notification
	@echo "Swagger documentation generated successfully!"
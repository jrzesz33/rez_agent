.PHONY: build build-scheduler build-processor build-webaction build-webapi clean deploy destroy help

# Variables
BUILD_DIR = build
INFRASTRUCTURE_DIR = infrastructure
AGENT_DIR = cmd/agent/*

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)rez_agent Makefile$(NC)"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}'

build: clean build-scheduler build-processor build-webaction build-webapi build-agent build-mcp ## Build all Lambda functions
	@echo "$(GREEN)All Lambda functions built successfully$(NC)"

build-scheduler: ## Build scheduler Lambda function
	@echo "$(YELLOW)Building scheduler Lambda...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/scheduler
	@cd $(BUILD_DIR) && zip scheduler.zip bootstrap && rm bootstrap
	@echo "$(GREEN)Scheduler Lambda built: $(BUILD_DIR)/scheduler.zip$(NC)"

build-processor: ## Build processor Lambda function
	@echo "$(YELLOW)Building processor Lambda...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/processor
	@cd $(BUILD_DIR) && zip processor.zip bootstrap && rm bootstrap
	@echo "$(GREEN)Processor Lambda built: $(BUILD_DIR)/processor.zip$(NC)"

build-webaction: ## Build webaction Lambda function
	@echo "$(YELLOW)Building webaction Lambda...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@cp pkg/courses/courseInfo.yaml $(BUILD_DIR)/courseInfo.yaml
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/webaction
	@cd $(BUILD_DIR) && zip webaction.zip bootstrap courseInfo.yaml && rm bootstrap && rm courseInfo.yaml
	@echo "$(GREEN)WebAction Lambda built: $(BUILD_DIR)/webaction.zip$(NC)"

build-webapi: ## Build webapi Lambda function
	@echo "$(YELLOW)Building webapi Lambda...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/webapi
	@cd $(BUILD_DIR) && zip webapi.zip bootstrap && rm bootstrap
	@echo "$(GREEN)WebAPI Lambda built: $(BUILD_DIR)/webapi.zip$(NC)"

build-agent: $(AGENT_DIR) ## Build AI agent Lambda function (Python)
	@rm -rf $(BUILD_DIR)/agent.zip
	@echo "$(YELLOW)Building AI agent Lambda...$(NC)"
	@mkdir -p $(BUILD_DIR)/agent
	@cp cmd/agent/*.py $(BUILD_DIR)/agent/
	@cp cmd/agent/*.json $(BUILD_DIR)/agent/
	@cp -r cmd/agent/ui $(BUILD_DIR)/agent/
	@if [ -d pkg ]; then cp -r pkg $(BUILD_DIR)/agent/; fi
	@echo "$(YELLOW)Installing Python dependencies using Docker with Lambda runtime (with cache)...$(NC)"
	@docker run --rm \
		--entrypoint pip \
		-v $(PWD)/cmd/agent/requirements.txt:/tmp/requirements.txt \
		-v $(PWD)/$(BUILD_DIR)/agent:/tmp/layer \
		-v rez-agent-pip-cache:/root/.cache/pip \
		public.ecr.aws/lambda/python:3.12 \
		install -r /tmp/requirements.txt -t /tmp/layer
	@echo "$(YELLOW)Optimizing package size...$(NC)"
	@find $(BUILD_DIR)/agent -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	@find $(BUILD_DIR)/agent -type d -name "tests" -exec rm -rf {} + 2>/dev/null || true
	@find $(BUILD_DIR)/agent -type d -name "*.dist-info" -exec rm -rf {} + 2>/dev/null || true
	@find $(BUILD_DIR)/agent -type f -name "*.pyc" -delete 2>/dev/null || true
	@find $(BUILD_DIR)/agent -type f -name "*.pyo" -delete 2>/dev/null || true
	@cd $(BUILD_DIR)/agent && zip -qr ../agent.zip .
	@rm -rf $(BUILD_DIR)/agent
	@echo "$(GREEN)AI Agent Lambda built: $(BUILD_DIR)/agent.zip$(NC)"

build-mcp: ## Build MCP Lambda function
	@echo "$(YELLOW)Building MCP Lambda...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@cp pkg/courses/courseInfo.yaml $(BUILD_DIR)/courseInfo.yaml
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/mcp
	@cd $(BUILD_DIR) && zip mcp.zip bootstrap courseInfo.yaml && rm bootstrap && rm courseInfo.yaml
	@echo "$(GREEN)MCP Lambda built: $(BUILD_DIR)/mcp.zip$(NC)"

build-mcp-client: ## Build MCP stdio client binary
	@echo "$(YELLOW)Building MCP stdio client...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/rez-agent-mcp-client ./tools/mcp-client
	@echo "$(GREEN)MCP client built: $(BUILD_DIR)/rez-agent-mcp-client$(NC)"

clean: ## Clean build artifacts (preserves pip cache)
	@echo "$(YELLOW)Cleaning build directory...$(NC)"
	@rm -rf $(BUILD_DIR)/mcp.zip $(BUILD_DIR)/scheduler.zip $(BUILD_DIR)/processor.zip $(BUILD_DIR)/webaction.zip $(BUILD_DIR)/webapi.zip 
	@echo "$(GREEN)Build directory cleaned (Docker pip cache preserved)$(NC)"

clean-all: ## Clean build artifacts including pip cache
	@echo "$(YELLOW)Cleaning build directory and Docker pip cache...$(NC)"
	@rm -rf $(BUILD_DIR)
	@docker volume rm rez-agent-pip-cache 2>/dev/null || true
	@echo "$(GREEN)Build directory and cache cleaned$(NC)"

test: ## Run all tests
	@echo "$(YELLOW)Running tests...$(NC)"
	@go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "$(YELLOW)Running tests with coverage...$(NC)"
	@go test -cover ./...
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

fmt: ## Format Go code
	@echo "$(YELLOW)Formatting Go code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)Code formatted$(NC)"

lint: ## Run linter (requires golangci-lint)
	@echo "$(YELLOW)Running linter...$(NC)"
	@golangci-lint run
	@echo "$(GREEN)Linting complete$(NC)"

tidy: ## Tidy Go modules
	@echo "$(YELLOW)Tidying Go modules...$(NC)"
	@go mod tidy
	@cd $(INFRASTRUCTURE_DIR) && go mod tidy
	@echo "$(GREEN)Modules tidied$(NC)"

# Infrastructure targets
infra-init: ## Initialize Pulumi infrastructure (run once)
	@echo "$(YELLOW)Initializing Pulumi infrastructure...$(NC)"
	@cd $(INFRASTRUCTURE_DIR) && pulumi login
	@echo "$(GREEN)Pulumi initialized$(NC)"

infra-preview: build ## Preview infrastructure changes
	@echo "$(YELLOW)Previewing infrastructure changes...$(NC)"
	@cd $(INFRASTRUCTURE_DIR) && pulumi preview

infra-up: build ## Deploy infrastructure with Pulumi
	@echo "$(YELLOW)Deploying infrastructure...$(NC)"
	@cd $(INFRASTRUCTURE_DIR) && pulumi up

infra-destroy: ## Destroy infrastructure
	@echo "$(RED)Destroying infrastructure...$(NC)"
	@cd $(INFRASTRUCTURE_DIR) && pulumi destroy

infra-stack-dev: ## Select dev stack
	@cd $(INFRASTRUCTURE_DIR) && pulumi stack select dev

infra-stack-prod: ## Select prod stack
	@cd $(INFRASTRUCTURE_DIR) && pulumi stack select prod

infra-outputs: ## Show infrastructure outputs
	@cd $(INFRASTRUCTURE_DIR) && pulumi stack output

# Local development targets
dev-env: ## Set up local development environment
	@echo "$(YELLOW)Setting up development environment...$(NC)"
	@go mod download
	@cd $(INFRASTRUCTURE_DIR) && go mod download
	@echo "$(GREEN)Development environment ready$(NC)"

local-dynamodb: ## Start local DynamoDB (requires Docker)
	@echo "$(YELLOW)Starting local DynamoDB...$(NC)"
	@docker run -d -p 8000:8000 --name dynamodb-local amazon/dynamodb-local
	@echo "$(GREEN)Local DynamoDB running on port 8000$(NC)"

local-dynamodb-stop: ## Stop local DynamoDB
	@echo "$(YELLOW)Stopping local DynamoDB...$(NC)"
	@docker stop dynamodb-local && docker rm dynamodb-local
	@echo "$(GREEN)Local DynamoDB stopped$(NC)"

# Deployment workflow
deploy-dev: build infra-stack-dev infra-up ## Build and deploy to dev environment
	@echo "$(GREEN)Deployment to dev complete$(NC)"

deploy-prod: build infra-stack-prod infra-up ## Build and deploy to prod environment
	@echo "$(GREEN)Deployment to prod complete$(NC)"

# Quick commands
quick-build: ## Quick build without cleaning
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/scheduler && cd $(BUILD_DIR) && zip -q scheduler.zip bootstrap && rm bootstrap
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/processor && cd $(BUILD_DIR) && zip -q processor.zip bootstrap && rm bootstrap
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(BUILD_DIR)/bootstrap ./cmd/webapi && cd $(BUILD_DIR) && zip -q webapi.zip bootstrap && rm bootstrap
	@echo "$(GREEN)Quick build complete$(NC)"

watch: ## Watch for changes and rebuild (requires entr)
	@echo "$(YELLOW)Watching for changes...$(NC)"
	@find . -name '*.go' | entr -c make quick-build

# Debug targets
lambda-logs-scheduler: ## Tail scheduler Lambda logs (requires AWS CLI and jq)
	@aws logs tail /aws/lambda/rez-agent-scheduler-$$(pulumi stack --cwd $(INFRASTRUCTURE_DIR) output | grep -o 'dev\|prod') --follow

lambda-logs-processor: ## Tail processor Lambda logs
	@aws logs tail /aws/lambda/rez-agent-processor-$$(pulumi stack --cwd $(INFRASTRUCTURE_DIR) output | grep -o 'dev\|prod') --follow

lambda-logs-webaction: ## Tail webaction Lambda logs
	@aws logs tail /aws/lambda/rez-agent-webaction-$$(pulumi stack --cwd $(INFRASTRUCTURE_DIR) output | grep -o 'dev\|prod') --follow

lambda-logs-webapi: ## Tail webapi Lambda logs
	@aws logs tail /aws/lambda/rez-agent-webapi-$$(pulumi stack --cwd $(INFRASTRUCTURE_DIR) output | grep -o 'dev\|prod') --follow

# Validation targets
validate: fmt test lint ## Run all validation checks
	@echo "$(GREEN)All validation checks passed$(NC)"

# Installation check
check-deps: ## Check if required dependencies are installed
	@echo "$(YELLOW)Checking dependencies...$(NC)"
	@command -v go >/dev/null 2>&1 || { echo "$(RED)Go is not installed$(NC)"; exit 1; }
	@command -v pulumi >/dev/null 2>&1 || { echo "$(RED)Pulumi is not installed$(NC)"; exit 1; }
	@command -v aws >/dev/null 2>&1 || { echo "$(RED)AWS CLI is not installed$(NC)"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "$(RED)Docker is not installed$(NC)"; exit 1; }
	@echo "$(GREEN)All required dependencies are installed$(NC)"

# SAM Local Testing targets
sam-validate: ## Validate SAM template
	@echo "$(YELLOW)Validating SAM template...$(NC)"
	@sam validate --lint
	@echo "$(GREEN)SAM template is valid$(NC)"

sam-build: build ## Build SAM application
	@echo "$(YELLOW)Building SAM application...$(NC)"
	@sam build
	@echo "$(GREEN)SAM build complete$(NC)"

sam-deploy-local: sam-build ## Deploy SAM stack to AWS (local testing)
	@echo "$(YELLOW)Deploying SAM stack to AWS...$(NC)"
	@sam deploy
	@echo "$(GREEN)SAM stack deployed$(NC)"

sam-start-api: sam-build docker-network-create ## Start local API Gateway
	@echo "$(YELLOW)Starting local API Gateway on http://localhost:3000$(NC)"
	@sam local start-api

sam-invoke-scheduler: sam-build docker-network-create ## Invoke scheduler function locally
	@echo "$(YELLOW)Invoking scheduler function...$(NC)"
	@sam local invoke SchedulerFunction --event events/scheduler-event.json

sam-invoke-webaction: sam-build docker-network-create ## Invoke webaction function locally
	@echo "$(YELLOW)Invoking webaction function...$(NC)"
	@sam local invoke WebActionFunction --event events/webaction-sqs-event.json

sam-invoke-webaction-golf: sam-build docker-network-create ## Invoke webaction function with golf event
	@echo "$(YELLOW)Invoking webaction function (golf)...$(NC)"
	@sam local invoke WebActionFunction --event events/webaction-golf-event.json

sam-invoke-processor: sam-build docker-network-create ## Invoke processor function locally
	@echo "$(YELLOW)Invoking processor function...$(NC)"
	@sam local invoke ProcessorFunction --event events/processor-sqs-event.json

sam-invoke-webapi: sam-build docker-network-create ## Invoke webapi function locally (uses AWS DynamoDB)
	@echo "$(YELLOW)Invoking webapi function...$(NC)"
	@echo "$(YELLOW)Note: This will use your AWS DynamoDB tables$(NC)"
	@sam local invoke WebApiFunction --event events/webapi-create-message.json

sam-invoke-mcp: sam-build docker-network-create ## Invoke MCP function locally
	@echo "$(YELLOW)Invoking MCP function...$(NC)"
	@sam local invoke McpFunction --event events/mcp-request.json

sam-delete: ## Delete SAM stack from AWS
	@echo "$(RED)Deleting SAM stack...$(NC)"
	@sam delete
	@echo "$(GREEN)SAM stack deleted$(NC)"

sam-logs: ## Tail SAM local logs
	@echo "$(YELLOW)Tailing SAM logs...$(NC)"
	@sam logs --tail

# DynamoDB Local targets
dynamodb-local-start: ## Start DynamoDB Local in Docker
	@echo "$(YELLOW)Starting DynamoDB Local...$(NC)"
	@docker run -d -p 8000:8000 --name dynamodb-local \
		--network rez-agent-local \
		amazon/dynamodb-local || echo "$(YELLOW)DynamoDB Local already running$(NC)"
	@echo "$(GREEN)DynamoDB Local running on http://localhost:8000$(NC)"

dynamodb-local-stop: ## Stop DynamoDB Local
	@echo "$(YELLOW)Stopping DynamoDB Local...$(NC)"
	@docker stop dynamodb-local || true
	@docker rm dynamodb-local || true
	@echo "$(GREEN)DynamoDB Local stopped$(NC)"

dynamodb-local-create-tables: ## Create local DynamoDB tables
	@echo "$(YELLOW)Creating local DynamoDB tables...$(NC)"
	@aws dynamodb create-table \
		--table-name rez-agent-messages-local \
		--attribute-definitions AttributeName=id,AttributeType=S \
		--key-schema AttributeName=id,KeyType=HASH \
		--billing-mode PAY_PER_REQUEST \
		--endpoint-url http://localhost:8000 \
		--region us-east-1 || echo "$(YELLOW)Messages table already exists$(NC)"
	@aws dynamodb create-table \
		--table-name rez-agent-schedules-local \
		--attribute-definitions AttributeName=id,AttributeType=S \
		--key-schema AttributeName=id,KeyType=HASH \
		--billing-mode PAY_PER_REQUEST \
		--endpoint-url http://localhost:8000 \
		--region us-east-1 || echo "$(YELLOW)Schedules table already exists$(NC)"
	@aws dynamodb create-table \
		--table-name rez-agent-web-action-results-local \
		--attribute-definitions AttributeName=id,AttributeType=S \
		--key-schema AttributeName=id,KeyType=HASH \
		--billing-mode PAY_PER_REQUEST \
		--endpoint-url http://localhost:8000 \
		--region us-east-1 || echo "$(YELLOW)Web action results table already exists$(NC)"
	@echo "$(GREEN)Local DynamoDB tables created$(NC)"

dynamodb-local-list-tables: ## List local DynamoDB tables
	@aws dynamodb list-tables \
		--endpoint-url http://localhost:8000 \
		--region us-east-1

# Docker network for local testing
docker-network-create: ## Create Docker network for local testing
	@echo "$(YELLOW)Creating Docker network...$(NC)"
	@docker network create rez-agent-local || echo "$(YELLOW)Network already exists$(NC)"
	@echo "$(GREEN)Docker network ready$(NC)"

docker-network-delete: ## Delete Docker network
	@echo "$(YELLOW)Deleting Docker network...$(NC)"
	@docker network rm rez-agent-local || true
	@echo "$(GREEN)Docker network deleted$(NC)"

# Complete local testing setup
local-setup: docker-network-create dynamodb-local-start dynamodb-local-create-tables ## Setup complete local testing environment
	@echo "$(GREEN)Local testing environment ready!$(NC)"
	@echo "$(GREEN)Run 'make sam-start-api' to start the API Gateway$(NC)"

local-teardown: dynamodb-local-stop docker-network-delete ## Teardown local testing environment
	@echo "$(GREEN)Local testing environment cleaned up$(NC)"

.DEFAULT_GOAL := help

.PHONY: build run test clean lint docker docker-run help

# Variables
APP_NAME := aiproxy
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-s -w -buildid= -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
GO := go
GOFLAGS := -v

# Directories
BIN_DIR := bin
DATA_DIR := data

# Default target
help:
	@echo "AIProxy - Multi-Account LLM API Gateway"
	@echo ""
	@echo "Usage:"
	@echo "  make build        Build optimized binary"
	@echo "  make run          Run the server"
	@echo "  make test         Run all tests"
	@echo "  make test-coverage Run tests with coverage"
	@echo "  make lint         Run linter"
	@echo "  make clean        Clean build artifacts"
	@echo "  make docker       Build Docker image"
	@echo "  make docker-run   Run Docker container"
	@echo "  make migrate-up   Run database migrations"
	@echo "  make dev          Run in development mode"

# Build
build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME) ./cmd/server
	@ls -lh $(BIN_DIR)/$(APP_NAME)

# Run
run: build
	@echo "Running $(APP_NAME)..."
	./$(BIN_DIR)/$(APP_NAME)

# Development mode
dev:
	@echo "Running in development mode..."
	$(GO) run ./cmd/server

# Test
test:
	@echo "Running tests..."
	$(GO) test -v -race ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Lint
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

# Clean
clean:
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR)
	@rm -f coverage.out coverage.html
	$(GO) clean

# Docker
docker:
	@echo "Building Docker image..."
	docker build -t $(APP_NAME):$(VERSION) -t $(APP_NAME):latest .

docker-run:
	@echo "Running Docker container..."
	docker run -d --name $(APP_NAME) \
		-p 8080:8080 \
		-p 8081:8081 \
		-v $(PWD)/data:/app/data \
		-v $(PWD)/config:/app/config \
		$(APP_NAME):latest

docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(APP_NAME) || true
	docker rm $(APP_NAME) || true

docker-compose-up:
	docker-compose up -d

docker-compose-down:
	docker-compose down

# Database
migrate-up:
	@echo "Running migrations..."
	@./scripts/migrate.sh up

migrate-down:
	@echo "Rolling back migrations..."
	@./scripts/migrate.sh down

# Initialize data directory
init:
	@mkdir -p $(DATA_DIR)
	@if [ ! -f config/config.json ]; then \
		cp config/config.example.json config/config.json; \
		echo "Created config/config.json from example"; \
	fi

# Install dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

# Format code
fmt:
	$(GO) fmt ./...

# Check
check: fmt lint test
	@echo "All checks passed!"
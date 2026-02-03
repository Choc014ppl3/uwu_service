.PHONY: build run test lint proto clean docker-build docker-run migrate-up migrate-down migrate-down-all migrate-force

# Build variables
BINARY_NAME=uwu_service
BUILD_DIR=bin
MAIN_PATH=./cmd/server

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet



# Docker variables
DOCKER_IMAGE=uwu_service
DOCKER_TAG=latest

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the application
build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

## run: Run the application
run:
	@echo "Running..."
	$(GOCMD) run $(MAIN_PATH)

## test: Run tests
test:
	@echo "Testing..."
	$(GOTEST) -v -race -cover ./...

## lint: Run linter
lint:
	@echo "Linting..."
	golangci-lint run ./...

## vet: Run go vet
vet:
	@echo "Vetting..."
	$(GOVET) ./...

## tidy: Tidy dependencies
tidy:
	@echo "Tidying..."
	$(GOMOD) tidy



## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)


## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f deployments/docker/Dockerfile .

## docker-run: Run with Docker Compose
docker-run:
	@echo "Running with Docker Compose..."
	docker-compose -f deployments/docker/docker-compose.yml up

## docker-down: Stop Docker Compose
docker-down:
	@echo "Stopping Docker Compose..."
	docker-compose -f deployments/docker/docker-compose.yml down

## dev: Run in development mode with hot reload (requires air)
dev:
	@echo "Running in development mode..."
	air -c .air.toml

## all: Build and test
all: tidy vet lint test build

# ==================== Migration Commands ====================

## migrate-up: Run all pending migrations
migrate-up:
	@echo "Running migrations up..."
	$(GOCMD) run ./cmd/migrate -direction=up -path=migrations

## migrate-down: Rollback last migration
migrate-down:
	@echo "Rolling back last migration..."
	$(GOCMD) run ./cmd/migrate -direction=down -steps=1 -path=migrations

## migrate-down-all: Rollback all migrations
migrate-down-all:
	@echo "Rolling back all migrations..."
	$(GOCMD) run ./cmd/migrate -direction=down -path=migrations

## migrate-force: Force migration version (use with VERSION=n)
migrate-force:
	@echo "Forcing migration version $(VERSION)..."
	$(GOCMD) run ./cmd/migrate -direction=force -steps=$(VERSION) -path=migrations


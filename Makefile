# Project variables
PROJECT_NAME := vpsie-k8s-autoscaler
BINARY_NAME := vpsie-autoscaler
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go variables
GOBASE := $(shell pwd)
GOBIN := $(GOBASE)/bin
GOFILES := $(wildcard *.go)

# Docker variables
DOCKER_REGISTRY := ghcr.io
DOCKER_IMAGE := $(DOCKER_REGISTRY)/vpsie/vpsie-k8s-autoscaler
DOCKER_TAG ?= $(VERSION)

# Kubernetes variables
NAMESPACE := kube-system

# Build flags
LDFLAGS := -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)

.PHONY: all build clean test lint help

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## all: Run build and test
all: clean build test

## build: Build the controller binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(GOBIN)
	@go build -ldflags "$(LDFLAGS)" -o $(GOBIN)/$(BINARY_NAME) ./cmd/controller

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(GOBIN)
	@rm -rf dist/
	@go clean

## test: Run unit tests
test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...

## test-integration: Run integration tests
test-integration:
	@echo "Running integration tests..."
	@go test -v -tags=integration ./test/integration/...

## test-e2e: Run end-to-end tests
test-e2e:
	@echo "Running E2E tests..."
	@go test -v -tags=e2e ./test/e2e/...

## lint: Run linters
lint:
	@echo "Running linters..."
	@$(HOME)/go/bin/golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@$(HOME)/go/bin/goimports -w .

## vet: Run go vet
vet:
	@go vet ./...

## generate: Generate code (CRDs, DeepCopy methods)
generate:
	@echo "Generating code..."
	@controller-gen object paths="./pkg/apis/autoscaler/v1alpha1/..."
	@controller-gen crd paths="./pkg/apis/autoscaler/v1alpha1/..." output:crd:dir=./deploy/crds

## manifests: Generate Kubernetes manifests
manifests:
	@echo "Generating manifests..."
	@controller-gen crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=deploy/manifests

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.

## docker-push: Push Docker image
docker-push:
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@docker push $(DOCKER_IMAGE):latest

## docker-login: Login to GitHub Container Registry
docker-login:
	@echo "Logging in to GitHub Container Registry..."
	@echo $(GITHUB_TOKEN) | docker login $(DOCKER_REGISTRY) -u $(GITHUB_USER) --password-stdin

## kind-create: Create kind cluster for development
kind-create:
	@echo "Creating kind cluster..."
	@kind create cluster --config deploy/kind/cluster.yaml --name $(PROJECT_NAME)
	@kubectl cluster-info --context kind-$(PROJECT_NAME)

## kind-delete: Delete kind cluster
kind-delete:
	@echo "Deleting kind cluster..."
	@kind delete cluster --name $(PROJECT_NAME)

## kind-load: Load Docker image into kind cluster
kind-load:
	@echo "Loading image into kind..."
	@kind load docker-image $(DOCKER_IMAGE):$(DOCKER_TAG) --name $(PROJECT_NAME)

## install: Install CRDs into cluster
install:
	@echo "Installing CRDs..."
	@kubectl apply -f deploy/crds/

## uninstall: Uninstall CRDs from cluster
uninstall:
	@echo "Uninstalling CRDs..."
	@kubectl delete -f deploy/crds/

## deploy: Deploy controller to cluster
deploy: install
	@echo "Deploying controller..."
	@kubectl apply -f deploy/manifests/deployment.yaml

## undeploy: Remove controller from cluster
undeploy:
	@echo "Removing controller..."
	@kubectl delete -f deploy/manifests/deployment.yaml

## run: Run controller locally
run:
	@echo "Running controller..."
	@go run ./cmd/controller/main.go

## helm-package: Package Helm chart
helm-package:
	@echo "Packaging Helm chart..."
	@helm package deploy/helm/vpsie-autoscaler -d dist/

## helm-install: Install via Helm
helm-install:
	@echo "Installing via Helm..."
	@helm install vpsie-autoscaler deploy/helm/vpsie-autoscaler \
		--namespace $(NAMESPACE) \
		--create-namespace

## helm-upgrade: Upgrade Helm release
helm-upgrade:
	@echo "Upgrading Helm release..."
	@helm upgrade vpsie-autoscaler deploy/helm/vpsie-autoscaler \
		--namespace $(NAMESPACE)

## helm-uninstall: Uninstall Helm release
helm-uninstall:
	@echo "Uninstalling Helm release..."
	@helm uninstall vpsie-autoscaler --namespace $(NAMESPACE)

## coverage: Generate coverage report
coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

## verify: Verify dependencies
verify:
	@echo "Verifying dependencies..."
	@go mod verify

.DEFAULT_GOAL := help

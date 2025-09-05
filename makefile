# Makefile for kubectl-ai
#
# This Makefile provides a set of commands to build, test, run,
# and manage the kubectl-ai project.

# Default target to run when no target is specified.
.DEFAULT_GOAL := help

# --- Variables ---
# Define common variables to avoid repetition and ease maintenance.
BIN_DIR      := ./bin
CMD_DIR      := ./cmd
BINARY_NAME  := kubectl-ai
BINARY_PATH  := $(BIN_DIR)/$(BINARY_NAME)

# Attempt to determine GOPATH/bin for installation.
# Fallback to a common default if `go env GOPATH` fails or is empty.
GOPATH_BIN   := $(shell go env GOPATH)/bin
ifeq ($(GOPATH_BIN),/bin)
	GOPATH_BIN := $(HOME)/go/bin
endif

# --- Environment Variables from .env ---
# If a .env file exists, include it. This makes variables defined in .env
# (e.g., API_KEY=123) available as Make variables.
# Then, export these variables so they are available in the environment
# for shell commands executed by Make recipes.
ifneq ($(wildcard .env),)
	include .env
	# Extract variable names from .env and export them.
	# This assumes .env contains lines like VAR=value.
	ENV_VARS_TO_EXPORT := $(shell awk -F= '{print $$1}' .env | xargs)
	export $(ENV_VARS_TO_EXPORT)
endif

# --- Help Target ---
# Displays a list of available targets and their descriptions.
# Descriptions are extracted from comments following '##'.
help:
	@echo "kubectl-ai Makefile"
	@echo "-------------------"
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# --- Build Tasks ---
build-recursive: ## Build the binary using dev script (recursive for all modules)
	@echo "λ Building all modules (recursive using dev script)..."
	mkdir -p $(BIN_DIR)
	./dev/ci/presubmits/go-build.sh

build: ## Build single binary for the current platform
	@echo "λ Building $(BINARY_NAME) for current platform..."
	mkdir -p $(BIN_DIR)
	go build -o $(BINARY_PATH) $(CMD_DIR)

# --- Run Tasks ---
run: ## Run the application
	@echo "λ Running $(BINARY_NAME) from source..."
	go run $(CMD_DIR)

run-html: ## Run with HTML UI
	@echo "λ Running $(BINARY_NAME) with HTML UI from source..."
	go run $(CMD_DIR) --user-interface html

# --- Code Quality Tasks (using dev scripts) ---
fmt: ## Format code using dev script
	@echo "λ Formatting code (using dev script)..."
	./dev/tasks/format.sh

vet: ## Run go vet using dev script
	@echo "λ Running go vet (using dev script)..."
	./dev/ci/presubmits/go-vet.sh

tidy: ## Tidy go modules using dev script
	@echo "λ Tidying go modules (using dev script)..."
	./dev/tasks/gomod.sh

# --- Verification Tasks (CI-style checks using dev scripts) ---
verify-format: ## Verify code formatting
	@echo "λ Verifying code formatting..."
	./dev/ci/presubmits/verify-format.sh

verify-gomod: ## Verify go.mod files are tidy
	@echo "λ Verifying go.mod files..."
	./dev/ci/presubmits/verify-gomod.sh

verify-autogen: ## Verify auto-generated files are up to date
	@echo "λ Verifying auto-generated files..."
	./dev/ci/presubmits/verify-autogen.sh

generate:
	go generate ./internal/mocks

verify-mocks:
	@echo "λ Verifying mocks..."
	./dev/ci/presubmits/verify-mocks.sh
# --- Generation Tasks ---
generate-actions: ## Generate GitHub Actions workflows
	@echo "λ Generating GitHub Actions workflows..."
	./dev/tasks/generate-github-actions.sh

# --- Evaluation Tasks ---
run-evals: ## Run evaluations (periodic task)
	@echo "λ Running evaluations..."
	./dev/ci/periodics/run-evals.sh

analyze-evals: ## Analyze evaluations (periodic task)
	@echo "λ Analyzing evaluations..."
	./dev/ci/periodics/analyze-evals.sh

# --- Combined Tasks ---
# 'check' depends on other verification tasks. They will run as prerequisites.
check: verify-format verify-gomod verify-autogen build-recursive vet ## Run all verification checks (presubmit-style)
	@echo "λ All checks completed."

# --- Development Workflow ---
# 'dev' and 'dev-html' depend on the 'build' target.
dev: build ## Development mode - build and run
	@echo "λ Starting $(BINARY_NAME) in dev mode..."
	$(BINARY_PATH)

dev-html: build ## Development mode - build and run with HTML UI
	@echo "λ Starting $(BINARY_NAME) with HTML UI in dev mode..."
	$(BINARY_PATH) --user-interface html

# --- Maintenance Tasks ---
clean: ## Clean build artifacts and coverage files
	@echo "λ Cleaning build artifacts..."
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

deps: ## Download Go module dependencies
	@echo "λ Downloading Go module dependencies..."
	go mod download

update-deps: ## Update Go module dependencies and then tidy
	@echo "λ Updating Go module dependencies..."
	go get -u ./...
	@echo "λ Tidying modules after update..."
	$(MAKE) tidy

# --- Installation ---
# 'install' depends on the 'build' target.
install: build ## Install the binary to $(GOPATH_BIN)
	@echo "λ Installing $(BINARY_NAME) to $(GOPATH_BIN)..."
	cp $(BINARY_PATH) $(GOPATH_BIN)/
	@echo "$(BINARY_NAME) installed."

# --- Testing ---
test: ## Run tests
	@echo "λ Running tests..."
	go test ./...

test-verbose: ## Run tests with verbose output
	@echo "λ Running tests (verbose)..."
	go test -v ./...

test-coverage: ## Run tests with coverage and generate HTML report
	@echo "λ Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	@echo "λ Generating coverage HTML report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"


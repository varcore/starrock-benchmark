APP_NAME    := starrock-benchmark
VERSION     ?= $(shell cat VERSION 2>/dev/null || echo "0.1.0")
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME)

GO          := go
GOFLAGS     := -trimpath
BINARY_DIR  := bin
DEB_DIR     := dist

# Detect host OS/arch for local builds
GOOS        ?= $(shell $(GO) env GOOS)
GOARCH      ?= $(shell $(GO) env GOARCH)

.PHONY: all build build-linux build-all test vet clean package-deb install uninstall version

all: build

# Build for current platform
build:
	@echo "Building $(APP_NAME) $(VERSION) ($(GOOS)/$(GOARCH))..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/$(APP_NAME) ./cmd/benchmark
	@echo "Built: $(BINARY_DIR)/$(APP_NAME)"

# Build for Linux amd64 (servers)
build-linux:
	@echo "Building $(APP_NAME) $(VERSION) (linux/amd64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/$(APP_NAME)-linux-amd64 ./cmd/benchmark
	@echo "Built: $(BINARY_DIR)/$(APP_NAME)-linux-amd64"

# Build for Linux arm64
build-linux-arm64:
	@echo "Building $(APP_NAME) $(VERSION) (linux/arm64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/$(APP_NAME)-linux-arm64 ./cmd/benchmark
	@echo "Built: $(BINARY_DIR)/$(APP_NAME)-linux-arm64"

# Build all platforms
build-all: build build-linux build-linux-arm64

# Run tests
test:
	$(GO) test ./...

# Run vet
vet:
	$(GO) vet ./...

# Build .deb package for linux/amd64
package-deb: build-linux
	@echo "Packaging .deb for $(APP_NAME) $(VERSION)..."
	@bash scripts/build-deb.sh $(VERSION) amd64
	@echo "Package: $(DEB_DIR)/$(APP_NAME)_$(VERSION)_amd64.deb"

# Build .deb package for linux/arm64
package-deb-arm64: build-linux-arm64
	@echo "Packaging .deb for $(APP_NAME) $(VERSION) (arm64)..."
	@bash scripts/build-deb.sh $(VERSION) arm64
	@echo "Package: $(DEB_DIR)/$(APP_NAME)_$(VERSION)_arm64.deb"

# Install locally (macOS/Linux)
install: build
	@echo "Installing $(APP_NAME) to /usr/local/bin/..."
	sudo cp $(BINARY_DIR)/$(APP_NAME) /usr/local/bin/$(APP_NAME)
	sudo chmod +x /usr/local/bin/$(APP_NAME)
	@echo "Installed. Run: $(APP_NAME) --config config.yaml"

uninstall:
	@echo "Removing $(APP_NAME)..."
	sudo rm -f /usr/local/bin/$(APP_NAME)
	@echo "Removed."

# Clean build artifacts
clean:
	rm -rf $(BINARY_DIR) $(DEB_DIR)
	@echo "Cleaned."

# Bump version: make bump-patch / bump-minor / bump-major
bump-patch:
	@bash scripts/bump-version.sh patch

bump-minor:
	@bash scripts/bump-version.sh minor

bump-major:
	@bash scripts/bump-version.sh major

# Print current version
version:
	@echo "$(APP_NAME) $(VERSION) (commit: $(GIT_COMMIT), built: $(BUILD_TIME))"

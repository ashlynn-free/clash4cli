# Version: use git tag if available, else VERSION file, else "dev"
# git describe --tags only succeeds if there are actual tags
VERSION ?= $(shell git describe --tags --dirty 2>/dev/null || cat VERSION 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
GO_BUILD_FLAGS := -trimpath -buildvcs=false

DIST_DIR := dist
BINARY_NAME := c4c
RGO_NAME := c4cgo
GO_CACHE_DIR := $(CURDIR)/.build/go-cache
GO_MOD_CACHE_DIR := $(CURDIR)/.build/go-mod-cache
GO_PROXY ?= https://proxy.golang.org,direct
GOENV := GOCACHE=$(GO_CACHE_DIR) GOMODCACHE=$(GO_MOD_CACHE_DIR) GOPROXY=$(GO_PROXY)
CURRENT_GOOS := $(shell go env GOOS)
CURRENT_GOARCH := $(shell go env GOARCH)
PROXYCHAINS_NG_VERSION ?= 4.17
MIHOMO_VERSION ?= 1.19.19
CGO_ENABLED ?= 0
.DEFAULT_GOAL := build

# Build targets: os/arch
TARGETS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64

.PHONY: all build clean package packages deps proxychains proxychains_all mihomo mihomo_all release $(TARGETS)

# Cross-platform: build all packages
all: packages

build:
	@echo "Building for current platform..."
	@mkdir -p $(GO_CACHE_DIR) $(GO_MOD_CACHE_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOENV) go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BINARY_NAME) ./cmd/c4c
	CGO_ENABLED=$(CGO_ENABLED) $(GOENV) go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(RGO_NAME) ./cmd/c4cgo
	@echo "Done: $(BINARY_NAME), $(RGO_NAME)"

# Build + create tar.gz for current platform
package:
	@$(MAKE) build-target GOOS=$(CURRENT_GOOS) GOARCH=$(CURRENT_GOARCH)

proxychains:
	@./scripts/fetch_proxychains.sh --version $(PROXYCHAINS_NG_VERSION)

proxychains_all:
	@./scripts/fetch_proxychains.sh --os darwin --arch amd64 --version $(PROXYCHAINS_NG_VERSION)
	@./scripts/fetch_proxychains.sh --os darwin --arch arm64 --version $(PROXYCHAINS_NG_VERSION)
	@./scripts/fetch_proxychains.sh --os linux --arch amd64 --version $(PROXYCHAINS_NG_VERSION)

mihomo:
	@./scripts/fetch_mihomo.sh --version $(MIHOMO_VERSION)

mihomo_all:
	@./scripts/fetch_mihomo.sh --os darwin --arch amd64 --version $(MIHOMO_VERSION)
	@./scripts/fetch_mihomo.sh --os darwin --arch arm64 --version $(MIHOMO_VERSION)
	@./scripts/fetch_mihomo.sh --os linux --arch amd64 --version $(MIHOMO_VERSION)

deps:
	@bash ./pull_deps.sh

# Build all platforms
release: clean $(TARGETS)
	@echo ""
	@echo "=== Release builds complete ==="
	@ls -lh $(DIST_DIR)/*.tar.gz 2>/dev/null || true
	@echo ""
	@echo "Folders:"
	@ls -d $(DIST_DIR)/clash4cli-*/ 2>/dev/null || true

# Alias: build all platform tar.gz packages
packages: release

# Cross-compile targets
darwin/amd64:
	@$(MAKE) build-target GOOS=darwin GOARCH=amd64

darwin/arm64:
	@$(MAKE) build-target GOOS=darwin GOARCH=arm64

linux/amd64:
	@$(MAKE) build-target GOOS=linux GOARCH=amd64

# Generic build target
build-target:
	$(eval OUTPUT_DIR := $(DIST_DIR)/clash4cli-$(GOOS)-$(GOARCH)-$(VERSION))
	$(eval ARCHIVE_NAME := clash4cli-$(GOOS)-$(GOARCH)-$(VERSION))
	@echo "Building $(GOOS)/$(GOARCH)..."
	@mkdir -p $(OUTPUT_DIR)

	@# Build binaries
	@mkdir -p $(GO_CACHE_DIR) $(GO_MOD_CACHE_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GOENV) go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)$(EXT) ./cmd/c4c
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GOENV) go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(OUTPUT_DIR)/$(RGO_NAME)$(EXT) ./cmd/c4cgo

	@# Include mihomo (required for c4c runtime)
	@if [ -f "third_party/mihomo/mihomo-$(GOOS)-$(GOARCH)$(EXT)" ]; then \
		cp -f third_party/mihomo/mihomo-$(GOOS)-$(GOARCH)$(EXT) $(OUTPUT_DIR)/mihomo$(EXT); \
		echo "  Included mihomo binary"; \
	elif [ -f "third_party/mihomo/mihomo$(EXT)" ] && [ "$(GOOS)" = "$$(go env GOOS)" ] && [ "$(GOARCH)" = "$$(go env GOARCH)" ]; then \
		cp -f third_party/mihomo/mihomo$(EXT) $(OUTPUT_DIR)/mihomo$(EXT); \
		echo "  Included mihomo binary"; \
	else \
		echo "  Error: mihomo binary missing for $(GOOS)/$(GOARCH)."; \
		echo "         Run: make mihomo_all (or make deps)"; \
		exit 1; \
	fi

	@# Copy proxychains if exists for this platform
	@PROXYCHAINS_DIR="third_party/proxychains/$(GOOS)-$(GOARCH)"; \
	if [ -d "$$PROXYCHAINS_DIR" ]; then \
		cp -f "$$PROXYCHAINS_DIR/proxychains4" $(OUTPUT_DIR)/ 2>/dev/null && echo "  Included proxychains4 binary" || true; \
		cp -f "$$PROXYCHAINS_DIR/libproxychains4.so" $(OUTPUT_DIR)/ 2>/dev/null && echo "  Included libproxychains4.so" || true; \
		cp -f "$$PROXYCHAINS_DIR/libproxychains4.dylib" $(OUTPUT_DIR)/ 2>/dev/null && echo "  Included libproxychains4.dylib" || true; \
		cp -f "$$PROXYCHAINS_DIR/"*.proxychains-ng $(OUTPUT_DIR)/ 2>/dev/null && echo "  Included proxychains-ng license files" || true; \
		cp -f "$$PROXYCHAINS_DIR/proxychains-ng.version" $(OUTPUT_DIR)/ 2>/dev/null && echo "  Included proxychains-ng version file" || true; \
	fi

	@# Include docs/version for convenience
	@cp -f README.md $(OUTPUT_DIR)/ 2>/dev/null || true
	@cp -f LICENSE $(OUTPUT_DIR)/ 2>/dev/null || true
	@cp -f THIRD_PARTY_NOTICES.md $(OUTPUT_DIR)/ 2>/dev/null || true
	@cp -f VERSION $(OUTPUT_DIR)/ 2>/dev/null || true

	@# Create archive
	tar -czf $(DIST_DIR)/$(ARCHIVE_NAME).tar.gz -C $(DIST_DIR) $(notdir $(OUTPUT_DIR))
	@echo "  Created $(ARCHIVE_NAME).tar.gz"

clean:
	@echo "Cleaning..."
	@rm -rf $(DIST_DIR)
	@rm -f $(BINARY_NAME) $(RGO_NAME)

# Install to /usr/local/bin
install: build
	@echo "Installing to /usr/local/bin..."
	sudo cp $(BINARY_NAME) /usr/local/bin/
	sudo cp $(RGO_NAME) /usr/local/bin/
	@echo "Done. Run 'c4c' to start."

# Uninstall
uninstall:
	@echo "Uninstalling..."
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	sudo rm -f /usr/local/bin/$(RGO_NAME)
	@echo "Done."

# Run dev
run:
	@mkdir -p $(GO_CACHE_DIR) $(GO_MOD_CACHE_DIR)
	$(GOENV) go run ./cmd/c4c

# Test
test:
	@mkdir -p $(GO_CACHE_DIR) $(GO_MOD_CACHE_DIR)
	$(GOENV) go test ./...

# Show version info
version:
	@echo "Version: $(VERSION)"
	@echo "Build time: $(BUILD_TIME)"

# Help
help:
	@echo "clash4cli build system"
	@echo ""
	@echo "Usage:"
	@echo "  make              Build for current platform (default)"
	@echo "  make build        Build for current platform"
	@echo "  make all          Build tar.gz for all platforms"
	@echo "  make package      Build + create tar.gz for current platform"
	@echo "  make packages     Alias of 'make all'"
	@echo "  make deps         Fetch all deps (mihomo + proxychains) for supported platforms"
	@echo "  make proxychains  Fetch/build proxychains-ng (current platform)"
	@echo "  make proxychains_all Fetch/build proxychains-ng (darwin amd64/arm64 + linux amd64)"
	@echo "  make mihomo       Fetch mihomo (current platform)"
	@echo "  make mihomo_all   Fetch mihomo (darwin amd64/arm64 + linux amd64)"
	@echo "  make release      Alias of 'make all' (kept for compatibility)"
	@echo "  make clean        Remove build artifacts"
	@echo "  make install      Install to /usr/local/bin"
	@echo "  make uninstall    Remove from /usr/local/bin"
	@echo "  make run          Run in development mode"
	@echo "  make test         Run tests"
	@echo "  make version      Show version info"
	@echo ""
	@echo "Individual platform builds:"
	@echo "  make darwin/amd64   macOS Intel"
	@echo "  make darwin/arm64   macOS Apple Silicon"
	@echo "  make linux/amd64    Linux x86_64"
	@echo ""
	@echo "Environment variables:"
	@echo "  VERSION           Override version string (default: git tag)"
	@echo "  CGO_ENABLED       Override CGO setting (default: 0)"

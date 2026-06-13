BINARY_NAME := pfcheck
PKG := github.com/rubiojr/pfcheck
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/cmd/pfcheck/util.Version=$(VERSION)

# Path where the pf-cli binary is staged for go:embed.
EMBED_DIR := internal/embedbin
EMBED_BIN := $(EMBED_DIR)/pf-cli

# How to build pf-cli: "docker" (Linux, in a container) or "native" (host
# CMake toolchain). Defaults to native on macOS (Docker would produce a Linux
# binary), docker elsewhere. Override with: make build PF_BUILD=native
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  PF_BUILD ?= native
else
  PF_BUILD ?= docker
endif

ifeq ($(PF_BUILD),native)
  PF_BUILD_SCRIPT := ./scripts/build-pf-native.sh
else
  PF_BUILD_SCRIPT := ./scripts/build-pf.sh
endif

.PHONY: all build build-noembed install pf-cli test fmt vet lint model clean

all: build

# Default build: embeds pf-cli so the result is a single self-contained binary
# (only the model is downloaded at runtime). Builds pf-cli first if needed.
build: $(EMBED_BIN)
	go build -tags embed_pfcli -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

# Build without embedding pf-cli (resolved from PATH / cache at runtime).
build-noembed:
	go build -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

install: $(EMBED_BIN)
	go install -tags embed_pfcli -ldflags "$(LDFLAGS)" ./cmd/$(BINARY_NAME)

# Build the privacy-filter.cpp pf-cli binary and stage it for embedding.
$(EMBED_BIN):
	$(PF_BUILD_SCRIPT) $(EMBED_DIR)

# Force a rebuild of the embedded pf-cli binary.
pf-cli:
	rm -f $(EMBED_BIN)
	$(PF_BUILD_SCRIPT) $(EMBED_DIR)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	staticcheck ./...

# Download the GGUF model into the pfcheck cache.
model: build
	./build/$(BINARY_NAME) download-model

clean:
	rm -rf build
	rm -f $(EMBED_BIN)

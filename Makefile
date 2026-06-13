BINARY_NAME := pfcheck
PKG := github.com/rubiojr/pfcheck
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/cmd/pfcheck/util.Version=$(VERSION)

# Path where the pf-cli binary is staged for go:embed. Built by scripts/build-pf.sh.
EMBED_DIR := internal/embedbin
EMBED_BIN := $(EMBED_DIR)/pf-cli

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

# Build the privacy-filter.cpp pf-cli binary in Docker and stage it for embedding.
$(EMBED_BIN):
	./scripts/build-pf.sh $(EMBED_DIR)

# Force a rebuild of the embedded pf-cli binary.
pf-cli:
	rm -f $(EMBED_BIN)
	./scripts/build-pf.sh $(EMBED_DIR)

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

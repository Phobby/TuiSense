APP_NAME    := tuisense
VERSION     ?= $(shell cat VERSION)
COMMIT      ?= $(shell git rev-parse HEAD 2>/dev/null)
PREFIX      ?= $(HOME)/.local
BINDIR      ?= $(PREFIX)/bin
BUILD_DIR   := bin
BINARY      := $(BUILD_DIR)/$(APP_NAME)
CMD_PKG     := ./cmd/tuisense
GO          ?= go
GOFLAGS     ?= -trimpath
LDFLAGS     := -s -w -X github.com/Phobby/TuiSense/internal/version.Version=$(VERSION) -X github.com/Phobby/TuiSense/internal/version.Commit=$(COMMIT)
WEB_DIR     := website/frontend
WEB_ADDR    ?= 127.0.0.1:8080

.PHONY: all build install uninstall test run mock udev media clean help web-build web-run web-test web-e2e

all: build

## build: Compile standalone TuiSense executable into ./bin/tuisense
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD_PKG)

## install: Build and install TuiSense into ~/.local/bin/tuisense (rootless)
install: build
	@PREFIX="$(PREFIX)" BINDIR="$(BINDIR)" ./scripts/install-from-source.sh

## uninstall: Remove ~/.local/bin/tuisense (preserves user profiles in ~/.config/tuisense)
uninstall:
	@PREFIX="$(PREFIX)" BINDIR="$(BINDIR)" ./scripts/install-from-source.sh --uninstall

## test: Run the complete unit and headless UI test suite
test:
	$(GO) test -v ./...

## run: Build and launch TuiSense (auto-detects real controller hardware)
run: build
	./$(BINARY)

## mock: Build and launch TuiSense in interactive mock DualSense mode
mock: build
	./$(BINARY) --mock

## udev: Display Linux udev rules for rootless /dev/hidraw* access
udev: build
	./$(BINARY) --print-udev

## media: Record the README demonstration GIFs from the real UI into docs/media
media:
	@rm -rf $(BUILD_DIR)/frames
	$(GO) run -ldflags "-X github.com/Phobby/TuiSense/internal/version.Version=$(VERSION)" ./tools/capture -out $(BUILD_DIR)/frames
	python3 tools/capture/render.py $(BUILD_DIR)/frames docs/media

## web-build: Build the web demo: tuisense, tuisense-webd and the website (same commit)
web-build: build
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/tuisense-webd ./website/backend/cmd/tuisense-webd
	cd $(WEB_DIR) && npm ci --no-audit --no-fund && npm run build

## web-run: Serve the web demo locally on http://$(WEB_ADDR) (Linux)
web-run: web-build
	TUISENSE_WEB_LISTEN=$(WEB_ADDR) TUISENSE_WEB_BINARY=$(CURDIR)/$(BINARY) \
	TUISENSE_WEB_STATIC_DIR=$(CURDIR)/$(WEB_DIR)/dist ./$(BUILD_DIR)/tuisense-webd

## web-test: Run the web demo's Go and frontend tests
web-test:
	$(GO) test ./internal/demo ./internal/app ./website/...
	cd $(WEB_DIR) && npm test

## web-e2e: Run the browser end-to-end test against a local tuisense-webd (needs Playwright)
web-e2e: web-build
	@set -e; \
	TUISENSE_WEB_LISTEN=127.0.0.1:8090 TUISENSE_WEB_BINARY=$(CURDIR)/$(BINARY) \
	TUISENSE_WEB_STATIC_DIR=$(CURDIR)/$(WEB_DIR)/dist ./$(BUILD_DIR)/tuisense-webd & pid=$$!; \
	trap "kill $$pid" EXIT; \
	for i in $$(seq 40); do curl -sf http://127.0.0.1:8090/healthz >/dev/null && break; sleep 0.25; done; \
	TUISENSE_E2E_URL=http://127.0.0.1:8090 node website/e2e/demo.e2e.cjs

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## help: Show available Makefile targets
help:
	@echo "TuiSense Makefile Targets:"
	@echo "  make build      Compile standalone executable to ./bin/tuisense"
	@echo "  make install    Install tuisense into ~/.local/bin and configure PATH"
	@echo "  make uninstall  Remove ~/.local/bin/tuisense"
	@echo "  make test       Run all unit and headless TUI tests"
	@echo "  make run        Build and run TuiSense"
	@echo "  make mock       Build and run TuiSense in interactive mock mode"
	@echo "  make udev       Print recommended Linux udev rules"
	@echo "  make media      Re-record the README GIFs into docs/media"
	@echo "  make web-run    Build and serve the browser demo locally (Linux)"
	@echo "  make web-test   Run the web demo's Go and frontend tests"
	@echo "  make web-e2e    Run the browser end-to-end test (needs Playwright)"
	@echo "  make clean      Remove compiled artifacts"

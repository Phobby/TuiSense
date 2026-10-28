APP_NAME    := tuisense
VERSION     ?= $(shell cat VERSION)
PREFIX      ?= $(HOME)/.local
BINDIR      ?= $(PREFIX)/bin
BUILD_DIR   := bin
BINARY      := $(BUILD_DIR)/$(APP_NAME)
CMD_PKG     := ./cmd/tuisense
GO          ?= go
GOFLAGS     ?= -trimpath
LDFLAGS     := -s -w -X github.com/Phobby/TuiSense/internal/version.Version=$(VERSION)

.PHONY: all build install uninstall test run mock udev media clean help

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
	@echo "  make clean      Remove compiled artifacts"

# Needs Go 1.20.x: newer Go builds binaries that do not run on macOS 10.13.
# Everything but "pkg" also works on Linux, since the build needs no cgo.

VERSION := $(shell cat VERSION)
GO ?= go
DIST := awg-hs-$(VERSION)-macos-x86_64
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test dist pkg clean check-go

all: build

check-go:
	@$(GO) version | grep -q ' go1\.20\.' || { echo "Go 1.20.x is required, found: $$($(GO) version)" >&2; exit 1; }

build: check-go
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/awg-hs ./cmd/awg-hs
	$(GO) run ./tools/minos build/awg-hs 10.13

test: check-go
	$(GO) vet ./...
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) vet ./cmd/... ./internal/...
	$(GO) test ./...

# Release archive: binary plus install/uninstall scripts.
dist: build
	rm -rf dist/$(DIST)
	mkdir -p dist/$(DIST)
	cp build/awg-hs packaging/install.sh packaging/uninstall.sh packaging/io.github.spark198rus.awg-hs.plist dist/$(DIST)/
	tar -C dist -czf dist/$(DIST).tar.gz $(DIST)

# Installer package; macOS only.
pkg: build
	packaging/build-pkg.sh $(VERSION)

clean:
	rm -rf build dist

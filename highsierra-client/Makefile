# Needs Go 1.20.x: newer Go builds binaries that do not run on macOS 10.13.
# "build", "test" and "dist" also work on Linux, since the service needs no
# cgo; "app" and "pkg" need macOS with Xcode.

VERSION := $(shell cat VERSION)
GO ?= go
DIST := awg-hs-$(VERSION)-macos-x86_64
LDFLAGS := -s -w -X main.version=$(VERSION)

APP := build/AWG-HS.app
APP_SRC := $(wildcard menubar/*.m)
APP_CFLAGS := -fobjc-arc -arch x86_64 -mmacosx-version-min=10.13 -O2 \
	-Wall -Wextra -Wno-unused-parameter \
	-Werror=unguarded-availability -Werror=unguarded-availability-new \
	-Werror=objc-method-access -Werror=incompatible-pointer-types

.PHONY: all build app test dist pkg clean check-go

all: build

check-go:
	@$(GO) version | grep -q ' go1\.20\.' || { echo "Go 1.20.x is required, found: $$($(GO) version)" >&2; exit 1; }

# The service and command-line tool.
build: check-go
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/awg-hs ./cmd/awg-hs
	$(GO) run ./tools/minos build/awg-hs 10.13

# The menu bar app. Any use of an API newer than macOS 10.13 without an
# availability check fails the build.
app: $(APP_SRC) menubar/*.h menubar/Info.plist
	rm -rf $(APP)
	mkdir -p $(APP)/Contents/MacOS
	clang $(APP_CFLAGS) -framework Cocoa -o $(APP)/Contents/MacOS/AWG-HS $(APP_SRC)
	sed 's/@VERSION@/$(VERSION)/g' menubar/Info.plist > $(APP)/Contents/Info.plist
	printf 'APPL????' > $(APP)/Contents/PkgInfo
	packaging/check-macho.sh $(APP)/Contents/MacOS/AWG-HS 10.13

test: check-go
	$(GO) vet ./...
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) vet ./cmd/... ./internal/...
	$(GO) test ./...

# Release archive: whatever is built (the service, and the app if "make app"
# ran) plus the install and uninstall scripts.
dist:
	@test -x build/awg-hs || { echo "build/awg-hs is missing: run make build first" >&2; exit 1; }
	rm -rf dist/$(DIST)
	mkdir -p dist/$(DIST)
	cp build/awg-hs packaging/install.sh packaging/uninstall.sh packaging/io.github.spark198rus.awg-hs.plist dist/$(DIST)/
	if [ -d $(APP) ]; then cp -R $(APP) dist/$(DIST)/; fi
	COPYFILE_DISABLE=1 tar -C dist -czf dist/$(DIST).tar.gz $(DIST)

# Installer package; macOS only.
pkg:
	packaging/build-pkg.sh $(VERSION)

clean:
	rm -rf build dist

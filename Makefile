.PHONY: ci ci-go ci-macos fmt fmt-check mod-tidy mod-tidy-check vet build test test-race swift-check swift-test scanner scanner-ci release verify-embedded-helper example clean

APP := WifiScanner.app
GO ?= go
SWIFTC ?= swiftc
GO_FILES := $(shell find . -name '*.go' -not -path './.git/*')
SWIFT_FILES := $(shell find scanner/Sources -name '*.swift' -type f | sort)
SWIFT_LIBRARY_FILES := $(shell find scanner/Sources -name '*.swift' -type f -not -name 'main.swift' | sort)
SWIFT_TEST_FILES := $(shell find scanner/Tests -name '*.swift' -type f | sort)

ci: ci-go

ci-go: fmt-check mod-tidy-check vet test-race build

ci-macos: ci-go swift-check swift-test scanner-ci

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@files="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "$$files"; \
		exit 1; \
	fi

mod-tidy:
	$(GO) mod tidy

mod-tidy-check:
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum

vet:
	$(GO) vet ./...

build:
	$(GO) build ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

swift-check:
	$(SWIFTC) -typecheck \
		-framework CoreWLAN \
		-framework CoreLocation \
		-framework Security \
		$(SWIFT_FILES)

swift-test:
	@set -e; \
	tmp="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	$(SWIFTC) -D TESTING -parse-as-library -Onone \
		-framework CoreWLAN \
		-framework CoreLocation \
		-framework Security \
		-o "$$tmp/WifiScannerTests" \
		$(SWIFT_LIBRARY_FILES) \
		$(SWIFT_TEST_FILES); \
	"$$tmp/WifiScannerTests"

# Local build: compile + sign with whatever cert is available.
# Output stays at the repo root, used via $MACWIFI_APP for dev iteration.
scanner: $(APP)

$(APP): $(SWIFT_FILES) scanner/Info.plist scanner/entitlements.plist scripts/build-wifi-scanner-app.sh scripts/wifi-scanner-source-digest.sh
	"$(CURDIR)/scripts/build-wifi-scanner-app.sh" --repo-root "$(CURDIR)" --bundle "$(CURDIR)/$(APP)"

scanner-ci:
	rm -rf $(APP)
	SIGN_IDENTITY=- $(MAKE) scanner

# Release build: sign with Developer ID, notarize, staple, stage for embed.
# Run this before cutting a new version of the Go module.
#   Prereqs (one-time): `xcrun notarytool store-credentials macwifi-notary ...`
release:
	"$(CURDIR)/scripts/release-wifi-scanner-app.sh" --repo-root "$(CURDIR)"

verify-embedded-helper:
	"$(CURDIR)/scripts/verify-wifi-scanner-app.sh" --repo-root "$(CURDIR)"

example: scanner
	MACWIFI_APP=$(PWD)/$(APP) go run ./examples/scan

clean:
	rm -rf $(APP)

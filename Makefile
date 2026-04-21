.PHONY: ci ci-go ci-macos fmt fmt-check mod-tidy mod-tidy-check vet build test test-race swift-check scanner scanner-ci release example clean

APP := WifiScanner.app
GO ?= go
SWIFTC ?= swiftc
GO_FILES := $(shell find . -name '*.go' -not -path './.git/*')

ci: ci-go

ci-go: fmt-check mod-tidy-check vet test-race build

ci-macos: ci-go swift-check scanner-ci

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
		scanner/Sources/main.swift

# Local build: compile + sign with whatever cert is available.
# Output stays at the repo root, used via $MACWIFI_APP for dev iteration.
scanner: $(APP)

$(APP): scanner/Sources/main.swift scanner/Info.plist scanner/entitlements.plist scanner/build.sh
	cd scanner && ./build.sh

scanner-ci:
	rm -rf $(APP)
	SIGN_IDENTITY=- $(MAKE) scanner

# Release build: sign with Developer ID, notarize, staple, stage for embed.
# Run this before cutting a new version of the Go module.
#   Prereqs (one-time): `xcrun notarytool store-credentials macwifi-notary ...`
release:
	cd scanner && ./release.sh

example: scanner
	MACWIFI_APP=$(PWD)/$(APP) go run ./examples/scan

clean:
	rm -rf $(APP)

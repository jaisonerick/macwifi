.PHONY: scanner release test example clean

APP := WifiScanner.app

# Local build: compile + sign with whatever cert is available.
# Output stays at the repo root, used via $MACWIFI_APP for dev iteration.
scanner: $(APP)

$(APP): scanner/Sources/main.swift scanner/Info.plist scanner/entitlements.plist scanner/build.sh
	cd scanner && ./build.sh

# Release build: sign with Developer ID, notarize, staple, stage for embed.
# Run this before cutting a new version of the Go module.
#   Prereqs (one-time): `xcrun notarytool store-credentials macwifi-notary ...`
release:
	cd scanner && ./release.sh

test:
	go test ./...

example: scanner
	MACWIFI_APP=$(PWD)/$(APP) go run ./examples/scan

clean:
	rm -rf $(APP)

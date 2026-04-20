.PHONY: scanner install test example clean

APP := WifiScanner.app
INSTALL_DIR := $(HOME)/.local/share/macwifi

scanner: $(APP)

$(APP): scanner/Sources/main.swift scanner/Info.plist scanner/entitlements.plist scanner/build.sh
	cd scanner && ./build.sh

install: scanner
	@install -d $(INSTALL_DIR)
	rm -rf $(INSTALL_DIR)/$(APP)
	cp -R $(APP) $(INSTALL_DIR)/
	@echo "→ installed $(APP) → $(INSTALL_DIR)/"
	@echo "  Scan() and Password() in Go consumers find it automatically."

test:
	go test ./...

example: scanner
	MACWIFI_APP=$(PWD)/$(APP) go run ./examples/scan

clean:
	rm -rf $(APP)

#!/usr/bin/env bash
# Build the signed WifiScanner.app. Output: ../WifiScanner.app
#
# Auto-picks Apple Development → Developer ID Application → Apple Distribution
# from the keychain. Override with SIGN_IDENTITY='full identity string'.

set -euo pipefail
cd "$(dirname "$0")"

APP_NAME="WifiScanner.app"
EXE_NAME="wifi-scanner"
OUT_DIR=".."
BUNDLE="$OUT_DIR/$APP_NAME"

if [[ -z "${SIGN_IDENTITY:-}" ]]; then
  # Prefer Developer ID for release/notarization; fall back to Apple
  # Development for local iteration on a dev Mac.
  for candidate in "Developer ID Application" "Apple Development" "Apple Distribution"; do
    match="$(security find-identity -v -p codesigning | awk -v candidate="$candidate" -F'"' '$2 ~ "^" candidate ":" {print $2; exit}')"
    if [[ -n "$match" ]]; then
      SIGN_IDENTITY="$match"
      break
    fi
  done
fi
if [[ -z "${SIGN_IDENTITY:-}" ]]; then
  echo "error: no codesigning identity found. Set SIGN_IDENTITY." >&2
  security find-identity -v -p codesigning | sed 's/^/  /' >&2
  exit 1
fi
echo "→ signing identity: $SIGN_IDENTITY"

rm -rf "$BUNDLE"
mkdir -p "$BUNDLE/Contents/MacOS"
cp Info.plist "$BUNDLE/Contents/Info.plist"

echo "→ compiling"
swiftc -O \
  -framework CoreWLAN \
  -framework CoreLocation \
  -framework Security \
  -o "$BUNDLE/Contents/MacOS/$EXE_NAME" \
  Sources/main.swift

echo "→ signing"
codesign --force \
  --sign "$SIGN_IDENTITY" \
  --options runtime \
  --entitlements entitlements.plist \
  "$BUNDLE"

codesign --verify --deep --strict "$BUNDLE"
echo "→ built $BUNDLE"

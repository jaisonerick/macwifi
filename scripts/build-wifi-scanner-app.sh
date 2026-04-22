#!/usr/bin/env bash
# Build and codesign WifiScanner.app.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(pwd)"
bundle=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo-root)
      repo_root="$2"
      shift 2
      ;;
    --bundle)
      bundle="$2"
      shift 2
      ;;
    *)
      echo "usage: $0 [--repo-root DIR] [--bundle PATH]" >&2
      exit 2
      ;;
  esac
done

repo_root="$(cd "$repo_root" && pwd)"
if [ -z "$bundle" ]; then
  bundle="$repo_root/WifiScanner.app"
fi

cd "$repo_root"

swiftc_bin="${SWIFTC:-swiftc}"
codesign_bin="${CODESIGN:-codesign}"
executable="$bundle/Contents/MacOS/wifi-scanner"
metadata="$bundle/Contents/Resources/macwifi-build.json"

if [ -z "${SIGN_IDENTITY:-}" ]; then
  for candidate in "Developer ID Application" "Apple Development" "Apple Distribution"; do
    match="$(security find-identity -v -p codesigning | awk -v candidate="$candidate" -F'"' '$2 ~ "^" candidate ":" {print $2; exit}')"
    if [ -n "$match" ]; then
      SIGN_IDENTITY="$match"
      break
    fi
  done
fi
if [ -z "${SIGN_IDENTITY:-}" ]; then
  echo "error: no codesigning identity found. Set SIGN_IDENTITY." >&2
  security find-identity -v -p codesigning | sed 's/^/  /' >&2
  exit 1
fi
echo "-> signing identity: $SIGN_IDENTITY"

digest="$("$script_dir/wifi-scanner-source-digest.sh" "$repo_root")"

rm -rf "$bundle"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cp scanner/Info.plist "$bundle/Contents/Info.plist"
python3 - "$metadata" "$digest" <<'PY'
import json
import sys

with open(sys.argv[1], "w", encoding="utf-8") as handle:
    json.dump({"sourceDigest": sys.argv[2]}, handle, indent=2)
    handle.write("\n")
PY

swift_sources=()
while IFS= read -r source; do
  swift_sources+=("$source")
done < <(find scanner/Sources -name '*.swift' -type f | LC_ALL=C sort)
if [ "${#swift_sources[@]}" -eq 0 ]; then
  echo "error: no Swift source files found under scanner/Sources" >&2
  exit 1
fi

# Pinned to match scanner/Info.plist LSMinimumSystemVersion. Without an
# explicit target, swiftc writes whatever the build-host SDK advertises
# into the LC_BUILD_VERSION load command, and macOS refuses to launch
# the helper on older hosts even when Info.plist claims support.
swift_target="${SWIFT_TARGET:-arm64-apple-macos13.0}"

echo "-> compiling (target $swift_target)"
"$swiftc_bin" -O \
  -target "$swift_target" \
  -framework CoreWLAN \
  -framework CoreLocation \
  -framework Security \
  -o "$executable" \
  "${swift_sources[@]}"

echo "-> signing"
"$codesign_bin" --force \
  --sign "$SIGN_IDENTITY" \
  --options runtime \
  --entitlements scanner/entitlements.plist \
  "$bundle"

"$codesign_bin" --verify --deep --strict "$bundle"

if [ -n "${EXPECTED_TEAM_ID:-}" ]; then
  team="$("$codesign_bin" -dv "$bundle" 2>&1 | awk -F= '/^TeamIdentifier=/ {print $2; exit}')"
  if [ "$team" != "$EXPECTED_TEAM_ID" ]; then
    echo "error: signed helper has TeamIdentifier=$team, expected $EXPECTED_TEAM_ID" >&2
    exit 1
  fi
fi

echo "-> built $bundle"

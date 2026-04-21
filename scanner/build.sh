#!/usr/bin/env bash
# Build the signed WifiScanner.app. Output: ../WifiScanner.app
#
# Auto-picks Apple Development → Developer ID Application → Apple Distribution
# from the keychain. Override with SIGN_IDENTITY='full identity string'.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

"$repo_root/scripts/build-wifi-scanner-app.sh" \
  --repo-root "$repo_root" \
  --bundle "$repo_root/WifiScanner.app"

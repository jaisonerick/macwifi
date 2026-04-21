#!/usr/bin/env bash
# Build + codesign + notarize + staple + stage for go:embed.
#
# Prereqs (one-time):
#   1. Developer ID Application cert installed in login keychain.
#   2. notarytool credentials stored under a profile name, e.g.:
#
#        xcrun notarytool store-credentials macwifi-notary \
#            --apple-id YOUR_APPLE_ID \
#            --team-id YOUR_TEAM_ID \
#            --password YOUR_APP_SPECIFIC_PASSWORD
#
# Override the profile name via NOTARY_PROFILE if you used a different one.
# CI can use direct notary credentials instead of a stored profile:
#   NOTARY_APPLE_ID, NOTARY_TEAM_ID, NOTARY_PASSWORD
# Override the signing identity via SIGN_IDENTITY if auto-detection picks the
# wrong cert.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

"$repo_root/scripts/release-wifi-scanner-app.sh" --repo-root "$repo_root"

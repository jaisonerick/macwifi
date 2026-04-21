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
cd "$(dirname "$0")"

APP="WifiScanner.app"
SCRATCH=".."             # where build.sh writes the initial bundle
EMBED_DIR="../embedded"
NOTARY_PROFILE="${NOTARY_PROFILE:-macwifi-notary}"
NOTARY_ARGS=()

if [[ -n "${NOTARY_APPLE_ID:-}" || -n "${NOTARY_TEAM_ID:-}" || -n "${NOTARY_PASSWORD:-}" ]]; then
  if [[ -z "${NOTARY_APPLE_ID:-}" || -z "${NOTARY_TEAM_ID:-}" || -z "${NOTARY_PASSWORD:-}" ]]; then
    echo "error: set NOTARY_APPLE_ID, NOTARY_TEAM_ID, and NOTARY_PASSWORD together" >&2
    exit 1
  fi
  NOTARY_ARGS=(--apple-id "$NOTARY_APPLE_ID" --team-id "$NOTARY_TEAM_ID" --password "$NOTARY_PASSWORD")
else
  NOTARY_ARGS=(--keychain-profile "$NOTARY_PROFILE")
fi

# ── 1. compile + sign via build.sh ─────────────────────────────────────────
./build.sh

CODESIGN_INFO=$(codesign -dvvv "$SCRATCH/$APP" 2>&1)
SIGN=$(printf '%s\n' "$CODESIGN_INFO" | awk -F'=' '/^Authority=/ {print $2; exit}')
if [[ "$SIGN" != "Developer ID Application"* ]]; then
  echo "error: built .app is signed with \"$SIGN\" — notarization requires Developer ID" >&2
  echo "       Set SIGN_IDENTITY to a Developer ID identity and rerun." >&2
  exit 1
fi
echo "→ leaf cert: $SIGN"

# ── 2. zip for notarization ────────────────────────────────────────────────
ZIP="/tmp/$APP.zip"
rm -f "$ZIP"
echo "→ packaging for notarization"
ditto -c -k --keepParent "$SCRATCH/$APP" "$ZIP"

# ── 3. submit & wait ───────────────────────────────────────────────────────
echo "→ submitting to Apple (this takes ~3–5 minutes)"
if ! xcrun notarytool submit "$ZIP" \
       "${NOTARY_ARGS[@]}" \
       --wait; then
  echo "error: notarization failed. Check the notarytool output above for the submission id." >&2
  if [[ "${NOTARY_ARGS[*]}" == *"--keychain-profile"* ]]; then
    echo "  xcrun notarytool history --keychain-profile $NOTARY_PROFILE" >&2
    echo "  xcrun notarytool log <submission-id> --keychain-profile $NOTARY_PROFILE" >&2
  fi
  exit 1
fi
rm -f "$ZIP"

# ── 4. staple the ticket ───────────────────────────────────────────────────
echo "→ stapling notarization ticket"
xcrun stapler staple "$SCRATCH/$APP"
xcrun stapler validate "$SCRATCH/$APP"

# ── 5. final spctl gatekeeper check ────────────────────────────────────────
echo "→ gatekeeper verification"
spctl -a -t exec -vv "$SCRATCH/$APP" 2>&1 | sed 's/^/  /'

# ── 6. stage for go:embed ─────────────────────────────────────────────────
echo "→ staging to $EMBED_DIR"
rm -rf "$EMBED_DIR/$APP"
mkdir -p "$EMBED_DIR"
cp -R "$SCRATCH/$APP" "$EMBED_DIR/"

echo
echo "✓ release build complete."
echo "  embedded artifact: $(cd "$EMBED_DIR" && pwd)/$APP"
echo "  next: commit the updated embedded helper, tag, and push."

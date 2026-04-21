#!/usr/bin/env bash
# Build, notarize, staple, and stage WifiScanner.app for go:embed.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(pwd)"
bundle=""
embedded_bundle=""

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
    --embedded-bundle)
      embedded_bundle="$2"
      shift 2
      ;;
    *)
      echo "usage: $0 [--repo-root DIR] [--bundle PATH] [--embedded-bundle PATH]" >&2
      exit 2
      ;;
  esac
done

repo_root="$(cd "$repo_root" && pwd)"
if [ -z "$bundle" ]; then
  bundle="$repo_root/WifiScanner.app"
fi
if [ -z "$embedded_bundle" ]; then
  embedded_bundle="$repo_root/embedded/WifiScanner.app"
fi

cd "$repo_root"

notary_profile="${NOTARY_PROFILE:-macwifi-notary}"
notary_args=()
if [ -n "${NOTARY_APPLE_ID:-}" ] || [ -n "${NOTARY_TEAM_ID:-}" ] || [ -n "${NOTARY_PASSWORD:-}" ]; then
  if [ -z "${NOTARY_APPLE_ID:-}" ] || [ -z "${NOTARY_TEAM_ID:-}" ] || [ -z "${NOTARY_PASSWORD:-}" ]; then
    echo "error: set NOTARY_APPLE_ID, NOTARY_TEAM_ID, and NOTARY_PASSWORD together" >&2
    exit 1
  fi
  notary_args=(--apple-id "$NOTARY_APPLE_ID" --team-id "$NOTARY_TEAM_ID" --password "$NOTARY_PASSWORD")
else
  notary_args=(--keychain-profile "$notary_profile")
fi

"$script_dir/build-wifi-scanner-app.sh" --repo-root "$repo_root" --bundle "$bundle"

codesign_info="$(codesign -dvvv "$bundle" 2>&1)"
leaf_cert="$(printf '%s\n' "$codesign_info" | awk -F= '/^Authority=/ {print $2; exit}')"
if [[ "$leaf_cert" != "Developer ID Application"* ]]; then
  echo "error: built .app is signed with \"$leaf_cert\"; notarization requires Developer ID" >&2
  echo "       Set SIGN_IDENTITY to a Developer ID identity and rerun." >&2
  exit 1
fi
echo "-> leaf cert: $leaf_cert"

zip_path="${RUNNER_TEMP:-/tmp}/WifiScanner.app.zip"
rm -f "$zip_path"
echo "-> packaging for notarization"
ditto -c -k --keepParent "$bundle" "$zip_path"

echo "-> submitting to Apple"
if ! xcrun notarytool submit "$zip_path" "${notary_args[@]}" --wait; then
  echo "error: notarization failed. Check the notarytool output above for the submission id." >&2
  if [[ "${notary_args[*]}" == *"--keychain-profile"* ]]; then
    echo "  xcrun notarytool history --keychain-profile $notary_profile" >&2
    echo "  xcrun notarytool log <submission-id> --keychain-profile $notary_profile" >&2
  fi
  exit 1
fi
rm -f "$zip_path"

echo "-> stapling notarization ticket"
xcrun stapler staple "$bundle"
xcrun stapler validate "$bundle"

echo "-> gatekeeper verification"
spctl -a -t exec -vv "$bundle"

echo "-> staging to $embedded_bundle"
rm -rf "$embedded_bundle"
mkdir -p "$(dirname "$embedded_bundle")"
cp -R "$bundle" "$embedded_bundle"

echo "-> release build complete"

#!/usr/bin/env bash
# Verify the staged signed WifiScanner.app matches current scanner sources.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(pwd)"
bundle=""
validate_staple=true

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
    --skip-stapler)
      validate_staple=false
      shift
      ;;
    *)
      echo "usage: $0 [--repo-root DIR] [--bundle PATH] [--skip-stapler]" >&2
      exit 2
      ;;
  esac
done

repo_root="$(cd "$repo_root" && pwd)"
if [ -z "$bundle" ]; then
  bundle="$repo_root/embedded/WifiScanner.app"
fi

cd "$repo_root"

metadata="$bundle/Contents/Resources/macwifi-build.json"
if [ ! -d "$bundle" ]; then
  echo "error: $bundle is missing" >&2
  exit 1
fi
if [ ! -f "$metadata" ]; then
  echo "error: $metadata is missing; rerun signing so the helper carries its source digest" >&2
  exit 1
fi

expected_digest="$("$script_dir/wifi-scanner-source-digest.sh" "$repo_root")"
actual_digest="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["sourceDigest"])' "$metadata")"
if [ "$actual_digest" != "$expected_digest" ]; then
  echo "error: embedded helper source digest is $actual_digest, expected $expected_digest" >&2
  exit 1
fi

codesign --verify --deep --strict "$bundle"

if [ -n "${EXPECTED_TEAM_ID:-}" ]; then
  team="$(codesign -dv "$bundle" 2>&1 | awk -F= '/^TeamIdentifier=/ {print $2; exit}')"
  if [ "$team" != "$EXPECTED_TEAM_ID" ]; then
    echo "error: embedded helper has TeamIdentifier=$team, expected $EXPECTED_TEAM_ID" >&2
    exit 1
  fi
fi

if [ "$validate_staple" = true ]; then
  xcrun stapler validate "$bundle"
fi

echo "-> verified $bundle"

#!/usr/bin/env bash
# Print a stable digest of the WifiScanner source inputs.

set -euo pipefail

repo_root="${1:-$(pwd)}"
repo_root="$(cd "$repo_root" && pwd)"

python3 - "$repo_root" <<'PY'
import hashlib
import os
import stat
import sys

root = sys.argv[1]
inputs = ["scanner/Info.plist", "scanner/entitlements.plist"]
source_root = os.path.join(root, "scanner", "Sources")

if not os.path.isdir(source_root):
    raise SystemExit("scanner/Sources is missing")

for dirpath, dirnames, filenames in os.walk(source_root, followlinks=False):
    for dirname in list(dirnames):
        path = os.path.join(dirpath, dirname)
        if os.path.islink(path):
            rel = os.path.relpath(path, root).replace(os.sep, "/")
            raise SystemExit(f"{rel} must not be a symlink")
    for filename in filenames:
        path = os.path.join(dirpath, filename)
        rel = os.path.relpath(path, root).replace(os.sep, "/")
        inputs.append(rel)

source_inputs = sorted(inputs[2:])
inputs = inputs[:2] + source_inputs

outer = hashlib.sha256()
for rel in inputs:
    path = os.path.join(root, rel)
    try:
        mode = os.lstat(path).st_mode
    except FileNotFoundError:
        raise SystemExit(f"{rel} is missing")
    if stat.S_ISLNK(mode):
        raise SystemExit(f"{rel} must not be a symlink")
    if not stat.S_ISREG(mode):
        raise SystemExit(f"{rel} must be a regular file")

    with open(path, "rb") as handle:
        digest = hashlib.sha256(handle.read()).hexdigest()
    outer.update(f"{rel}\t{digest}\n".encode("utf-8"))

print(outer.hexdigest())
PY

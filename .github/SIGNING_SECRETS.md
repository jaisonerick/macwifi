# Signing Secrets

The `Signed macOS Companion` workflow runs on pushes to `main` that touch the
Swift companion. It imports a Developer ID Application certificate, builds the
helper, notarizes it with Apple, staples the ticket, and opens a generated pull
request that updates `embedded/WifiScanner.app`.

The workflow does not run for `embedded/WifiScanner.app` changes. Generated
helper PRs can merge without retriggering the signing workflow, which keeps the
signed bundle in the Go module without creating a workflow loop.

Required repository secrets:

- `MACOS_CERTIFICATE_P12_BASE64`: base64-encoded Developer ID Application
  `.p12` certificate.
- `MACOS_CERTIFICATE_PASSWORD`: password for the `.p12` certificate.
- `APPLE_ID`: Apple ID email used for notarization.
- `APPLE_TEAM_ID`: Apple Developer team ID.
- `APPLE_APP_SPECIFIC_PASSWORD`: app-specific password for the Apple ID.

Optional repository secret:

- `MACOS_CODESIGN_IDENTITY`: full Developer ID identity string. Leave unset to
  let `scanner/build.sh` pick the first Developer ID Application identity from
  the imported certificate.
- `MACWIFI_AUTOMATION_TOKEN`: GitHub token with `repo` and `workflow` scopes.
  When set, generated helper PR branches are pushed with this token so their CI
  runs are created normally and auto-merge can complete.

Example setup:

```sh
security export \
  -k login.keychain-db \
  -t identities \
  -f pkcs12 \
  -o developer-id.p12

base64 -i developer-id.p12 | gh secret set MACOS_CERTIFICATE_P12_BASE64
gh secret set MACOS_CERTIFICATE_PASSWORD
gh secret set APPLE_ID
gh secret set APPLE_TEAM_ID
gh secret set APPLE_APP_SPECIFIC_PASSWORD
```

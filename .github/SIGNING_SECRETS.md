# Signing Secrets

The `Signed macOS Companion` workflow runs on pushes to `main` that touch the
Swift companion. It imports a Developer ID Application certificate, builds the
helper, notarizes it with Apple, staples the ticket, and opens a generated pull
request that updates `embedded/WifiScanner.app`.

The workflow does not run for `embedded/WifiScanner.app` changes. Generated
helper PRs can merge without retriggering the signing workflow, which keeps the
signed bundle in the Go module without creating a workflow loop.

The signing secrets should be stored on the protected `macos-signing`
environment, not as repository-wide secrets. That environment should be limited
to protected branches and require approval before deployment. This keeps pull
request workflows from reading signing material, and it adds a manual gate
before any changed signing workflow can access the private key or notary
password on `main`.

Required `macos-signing` environment secrets:

- `MACOS_CERTIFICATE_P12_BASE64`: base64-encoded Developer ID Application
  `.p12` certificate.
- `MACOS_CERTIFICATE_PASSWORD`: password for the `.p12` certificate.
- `APPLE_ID`: Apple ID email used for notarization.
- `APPLE_TEAM_ID`: Apple Developer team ID.
- `APPLE_APP_SPECIFIC_PASSWORD`: app-specific password for the Apple ID.

Optional `macos-signing` environment secrets:

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

base64 -i developer-id.p12 | gh secret set MACOS_CERTIFICATE_P12_BASE64 --env macos-signing
gh secret set MACOS_CERTIFICATE_PASSWORD --env macos-signing
gh secret set APPLE_ID --env macos-signing
gh secret set APPLE_TEAM_ID --env macos-signing
gh secret set APPLE_APP_SPECIFIC_PASSWORD --env macos-signing
```

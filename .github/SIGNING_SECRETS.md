# Signing Secrets

The `Signed macOS Companion` workflow is run manually with a pull request
number after a maintainer has reviewed companion changes. It checks out that PR
branch, imports a Developer ID Application certificate, builds the helper,
notarizes it with Apple, staples the ticket, and pushes the updated
`embedded/WifiScanner.app` back to the same PR branch.

The workflow refuses to push signing output to fork PRs. If the signed output
matches the branch, it exits without changing anything. If it is run without a
PR number and the signed output differs from `main`, it fails instead of opening
a second generated PR.

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

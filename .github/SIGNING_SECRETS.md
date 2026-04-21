# Signing Secrets

The `Signed macOS Companion` workflow runs automatically for pull requests that
target `main`. When companion source changes, it builds the helper from trusted
workflow steps, signs it with Developer ID, notarizes it, staples the ticket,
and pushes the updated `embedded/WifiScanner.app` back to the same pull request
branch. The generated commit is then verified by a second automatic workflow run
before the final `Signed companion` check passes.

This keeps the signed helper in the pull request before merge. `main` should
never need a second generated commit after a companion change merges.

## Security model

- The workflow uses `pull_request_target`, so pull requests cannot change the
  signing workflow that runs for their own branch.
- The secret-bearing job refuses companion changes from fork pull requests.
- The signing job checks out pull request files, but it does not run pull
  request-controlled shell scripts. It compiles `scanner/Sources`, copies
  `scanner/Info.plist`, and signs with the fixed commands in the workflow.
- The signed app contains `Contents/Resources/macwifi-build.json` with a digest
  of the companion source inputs. Verification compares that digest against the
  current pull request source, verifies code signing, checks the Apple team ID,
  and validates the stapled notarization ticket.
- Branch protection should require the final `Signed companion` check together
  with the normal CI checks. That blocks merges until the latest pull request
  commit already contains the matching signed helper.

The signing secrets should be stored on the protected `macos-signing`
environment, not as repository-wide secrets. Keep the environment limited to
protected branches. Do not require manual environment reviewers if signing must
run automatically for pull requests; a reviewer gate will pause the signing job
and prevent the required `Signed companion` check from completing.

Required `macos-signing` environment secrets:

- `MACOS_CERTIFICATE_P12_BASE64`: base64-encoded Developer ID Application
  `.p12` certificate.
- `MACOS_CERTIFICATE_PASSWORD`: password for the `.p12` certificate.
- `APPLE_ID`: Apple ID email used for notarization.
- `APPLE_TEAM_ID`: Apple Developer team ID.
- `APPLE_APP_SPECIFIC_PASSWORD`: app-specific password for the Apple ID.
- `MACWIFI_AUTOMATION_TOKEN`: GitHub token with `repo` and `workflow` scopes.
  This token is required so the generated commit triggers the follow-up
  verification workflow run.

Optional `macos-signing` environment secrets:

- `MACOS_CODESIGN_IDENTITY`: full Developer ID identity string. Leave unset to
  let the workflow pick the first imported Developer ID Application identity.

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
gh secret set MACWIFI_AUTOMATION_TOKEN --env macos-signing
```

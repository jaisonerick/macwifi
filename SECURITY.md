# Security Policy

`macwifi` interacts with macOS Location Services and Keychain prompts, so
security and privacy reports are especially important.

## Supported Versions

Security fixes are handled on the latest released version of the module.

## Reporting A Vulnerability

Do not open a public issue with vulnerability details.

Use GitHub private vulnerability reporting if it is available for this
repository. If private reporting is not available, open a public issue asking
for a private contact channel and do not include exploit details, private SSIDs,
BSSIDs, passwords, logs, or screenshots.

Please include:

- A short description of the vulnerability.
- Impact and affected versions, if known.
- Reproduction steps or proof-of-concept details.
- Any relevant macOS and Go versions.
- Whether the issue involves scan data, Location Services, Keychain access, the
  embedded helper, or release signing.

## Security Boundaries

The project will not treat the following as vulnerabilities by themselves:

- macOS asking for Location Services before returning unredacted WiFi data.
- macOS asking for Keychain approval before returning a saved WiFi password.
- Keychain prompts appearing repeatedly for password access.

Those behaviors are part of the platform security model. Reports about
accidental disclosure, prompt confusion, permission escalation, or incorrect
handling of denied permissions are still welcome.

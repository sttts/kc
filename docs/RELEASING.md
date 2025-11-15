# Releasing kc

This repo uses [Goreleaser](https://goreleaser.com) to produce cross-platform tarballs, publish GitHub Releases, and update the Homebrew tap in [`sttts/homebrew-kc`](https://github.com/sttts/homebrew-kc).

## Prerequisites

1. Install Goreleaser locally (`brew install goreleaser` or download a binary).
2. Import the GPG key used for signing, and store its passphrase in a secure location.
3. Ensure your git workspace is clean and tagged (`git tag v0.1.0 && git push origin v0.1.0`). Pushing the tag triggers the release workflow automatically.

## CI automation

Every push of a `v*` tag triggers `.github/workflows/release.yml`, which:

1. Builds linux/darwin binaries for amd64/arm64.
2. Archives each build, generates checksums, and signs both archives and checksum files with the configured GPG key.
3. Publishes the GitHub Release for the tag.
4. Updates `sttts/homebrew-kc` with the refreshed formula so `brew install sttts/homebrew-kc/kc` picks up the new version.

### Required secrets

Configure these repository secrets for the workflow:

| Secret | Purpose |
| --- | --- |
| `RELEASE_GITHUB_TOKEN` | Personal access token with `repo` scope; needed to create releases and push to `sttts/homebrew-kc`. |
| `RELEASE_GPG_PRIVATE_KEY` | ASCII-armored private key used for signing archives/checksums. |
| `RELEASE_GPG_PASSPHRASE` | Passphrase for the key above (leave empty if the key is unencrypted). |
| `MACOS_SIGN_P12` | Base64 of the exported Developer ID Application `.p12` certificate. |
| `MACOS_SIGN_PASSWORD` | Password used when exporting the `.p12` file. |
| `MACOS_NOTARY_KEY` | Base64 of the App Store Connect API key (`.p8`). |
| `MACOS_NOTARY_KEY_ID` | The key ID of the App Store Connect API key. |
| `MACOS_NOTARY_ISSUER_ID` | Issuer ID (UUID) from App Store Connect for that API key. |

### Apple signing & notarization

macOS binaries are signed with a Developer ID Application certificate and then notarized via App Store Connect so Gatekeeper trusts the downloaded archives. To configure this:

1. In Apple Developer portal create/download a **Developer ID Application** certificate. Import it into Keychain, export it as a password-protected `.p12`, then base64-encode it (`base64 -w0 Certificates.p12 > macos_sign_p12.txt`). Store the contents in `MACOS_SIGN_P12` and the export password in `MACOS_SIGN_PASSWORD`.
2. In App Store Connect create an **API key** with download/notarization access. Note the `Issuer ID` and `Key ID`, and base64-encode the downloaded `.p8` (`base64 -w0 ApiKey_ABC123.p8 > macos_notary_key.txt`). Store those values in `MACOS_NOTARY_KEY`, `MACOS_NOTARY_KEY_ID`, and `MACOS_NOTARY_ISSUER_ID`.
3. Once configured, every tagged release will codesign and notarize the macOS binaries. After installation you can verify with `spctl -a -vv /opt/homebrew/bin/kc`.

### Verification / dry runs

Before cutting a tag, run `goreleaser release --snapshot --clean` locally. It exercises the same pipeline without touching GitHub or Homebrew. Ensure `GPG_PASSPHRASE_FILE` and the key are available locally when doing so.

## Publishing manually (optional)

If you ever need to rebuild a release locally (e.g., CI outage), run:

```bash
GORELEASER_GITHUB_TOKEN=<token> GPG_PASSPHRASE_FILE=/tmp/gpg-passphrase goreleaser release --clean
```

Make sure the GPG key is imported (`gpg --import`) and `/tmp/gpg-passphrase` contains the passphrase text.

## Formula updates

After the release finishes, users can install via:
```bash
brew tap sttts/homebrew-kc
brew install sttts/homebrew-kc/kc
```

Keep this document updated whenever the release process changes.

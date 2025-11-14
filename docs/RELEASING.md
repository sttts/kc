# Releasing kc

This repo uses [Goreleaser](https://goreleaser.com) to produce cross-platform tarballs, publish GitHub Releases, and update the Homebrew tap in [`sttts/homebrew-kc`](https://github.com/sttts/homebrew-kc).

## Prerequisites

1. Install Goreleaser locally (`brew install goreleaser` or download a binary).
2. Export a `GITHUB_TOKEN` with `repo` scope that can push to both `sttts/kc` and `sttts/homebrew-kc`.
3. Ensure your git workspace is clean and tagged (`git tag v0.1.0 && git push origin v0.1.0`).

## Dry run

Use `goreleaser release --snapshot --clean` to verify builds and tap updates without touching GitHub.

## Publish a release

```bash
goreleaser release --clean
```

This command:
- Builds `kc` for linux/darwin on amd64/arm64 with embedded version metadata.
- Creates tarballs plus a checksum file and uploads them to the GitHub Release for the current tag.
- Updates `sttts/homebrew-kc` with a formula that references the new artifacts (committing as `kc release bot`).

If anything fails, fix the issue and rerun `goreleaser release --clean`; Goreleaser will reuse the existing tag and update artifacts/tap as needed.

## Formula updates

After the release finishes, users can install via:
```bash
brew tap sttts/homebrew-kc
brew install sttts/homebrew-kc/kc
```

Keep this document updated whenever the release process changes.

# Developer Guide: Releases and Distribution

## Prerequisites
- GitHub Actions enabled on this repo
- Optional (for package managers):
  - Secret `GH_PAT` with a Personal Access Token that has access to:
    - `stigoleg/homebrew-tap`
    - `stigoleg/scoop-bucket`
  - Secret `PUBLISH_PACKAGE_MANAGERS` set to `true`
- Optional (macOS signing/notarization): set secrets used in `.github/workflows/release.yml` (MACOS_*).

## Normal release flow (automated)
1. Update version in `cmd/keepalive/main.go` (e.g., `const appVersion = "1.3.2"`).
2. Commit to `main`.
3. Tag and push:
   ```bash
   git tag -a v1.3.2 -m "Release v1.3.2"
   git push origin v1.3.2
   ```
4. GitHub Actions will run `release.yml`:
   - Build archives for macOS, Linux, Windows
   - Upload artifacts to the GitHub release
   - If `PUBLISH_PACKAGE_MANAGERS=true` and `GH_PAT` is set:
     - Update `homebrew-tap` with a new `Formula/keepalive.rb`
     - Update `scoop-bucket` with `keepalive.json`

## Re-trigger a release for the same version
If you fix CI config after tagging, move the tag to the latest commit and force-push:
```bash
# move the existing tag to the current commit
git tag -f v1.3.2
git push --force origin v1.3.2
```

## Manual Homebrew update (fallback)
If auto-publish is not configured:
1. Ensure release artifacts for macOS are published (Darwin x86_64 and arm64 tarballs).
2. Clone `homebrew-tap`:
   ```bash
   git clone git@github.com:stigoleg/homebrew-tap.git
   cd homebrew-tap/Formula
   ```
3. Edit `keepalive.rb`:
   - Update `url` to point to the new release tarball(s)
   - Update `sha256` for each architecture
   - Update version if present
4. Compute sha256 locally if needed:
   ```bash
   shasum -a 256 ~/Downloads/keep-alive_Darwin_x86_64.tar.gz | awk '{print $1}'
   shasum -a 256 ~/Downloads/keep-alive_Darwin_arm64.tar.gz | awk '{print $1}'
   ```
5. Commit and push:
   ```bash
   git add keepalive.rb
   git commit -m "keepalive: bump to v1.3.2"
   git push
   ```

## Manual Scoop update (fallback)
1. Ensure release artifacts for Windows are published (zip x86_64).
2. Clone `scoop-bucket`:
   ```bash
   git clone git@github.com:stigoleg/scoop-bucket.git
   cd scoop-bucket
   ```
3. Edit `keepalive.json`:
   - Update `version`, `url`, and `hash` (sha256 of the new zip)
4. Compute sha256:
   ```bash
   shasum -a 256 ~/Downloads/keep-alive_Windows_x86_64.zip | awk '{print $1}'
   ```
5. Commit and push:
   ```bash
   git add keepalive.json
   git commit -m "keepalive: bump to v1.3.2"
   git push
   ```

## Troubleshooting
- Release job fails on Homebrew/Scoop templating:
  - Ensure `.goreleaser.yml` uses `skip_upload: '{{ not (isEnvSet "PUBLISH_PACKAGE_MANAGERS") }}'`.
  - Set repo secrets: `GH_PAT` and `PUBLISH_PACKAGE_MANAGERS=true`.
- Need to re-run a failed release for an existing tag:
  - Move the tag to the latest commit and force-push (see above).
- No macOS notarization:
  - Expected if MACOS_* secrets are not set; artifacts still build and run.

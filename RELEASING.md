# Releasing keepalive

Releases are built, signed, notarized and published from the maintainer's Mac
with `make release`. The Developer ID lives in the login keychain, so GitHub
Actions only runs CI. Pushing a tag does not start any workflow.

## One-time setup

1. **Tools:** Xcode or the Command Line Tools (`codesign`, `xcrun notarytool`),
   and GoReleaser v2: `brew install goreleaser`. Docker is optional, for
   checking the Linux packages.

2. **Signing identity.** This command must list
   `Developer ID Application: Stig-Ole Gundersen (49P95434EE)`:

   ```sh
   security find-identity -v -p codesigning
   ```

   If codesign ever asks for keychain access, choose *Always Allow*.
   Otherwise an unattended release stalls on the dialog.

3. **Notary profile.** Store the notarization credentials in the keychain
   once, under the profile name `keepalive-notary`. With an Apple ID, it
   prompts for an app-specific password, which you create at
   account.apple.com → Sign-In and Security → App-Specific Passwords:

   ```sh
   xcrun notarytool store-credentials keepalive-notary \
     --apple-id <apple-id-email> --team-id 49P95434EE
   ```

   Or with an App Store Connect API key:

   ```sh
   xcrun notarytool store-credentials keepalive-notary \
     --key AuthKey_<KEYID>.p8 --key-id <KEYID> --issuer <issuer-uuid>
   ```

   Check it with `xcrun notarytool history --keychain-profile keepalive-notary`.
   To use another profile name, set `KEEPALIVE_NOTARY_PROFILE`.

4. **Tokens.**
   - `GITHUB_TOKEN`: lets GoReleaser create the release and upload assets to
     `stigoleg/keep-alive`. Use a fine-grained token with *Contents: read and
     write* on that repository, or `export GITHUB_TOKEN=$(gh auth token)`.
   - `GH_PAT`: pushes the cask to `stigoleg/homebrew-tap` and the manifest to
     `stigoleg/scoop-bucket`. Use a fine-grained token with *Contents: read
     and write* on both repositories. It is only used when
     `PUBLISH_PACKAGE_MANAGERS` is set.

5. **Homebrew formula to cask (2.0.0 only).** 1.x was
   `Formula/keepalive.rb`; 2.x publishes `Casks/keepalive.rb`, because
   GoReleaser no longer generates formulas. After the first cask is
   published, delete `Formula/keepalive.rb` from the tap. Existing users run
   `brew uninstall keepalive && brew install --cask stigoleg/tap/keepalive`.

## Every release

1. Start from an up-to-date `main` with green CI.
2. In `CHANGELOG.md`, change `## [X.Y.Z] - Unreleased` to
   `## [X.Y.Z] - YYYY-MM-DD`, then commit it. That section becomes the
   header of the release notes. GoReleaser appends the commits, grouped by
   conventional-commit type.
3. Dry run: `make snapshot`, then check `dist/` (see Verification).
4. Tag the commit and push the tag. GoReleaser releases the tag that origin
   has; without it, GitHub would create the tag on the default branch.

   ```sh
   git tag -a vX.Y.Z -m "keepalive X.Y.Z"
   git push origin main vX.Y.Z
   ```

5. Release:

   ```sh
   export GITHUB_TOKEN=…
   export PUBLISH_PACKAGE_MANAGERS=1 GH_PAT=…   # to update Homebrew and Scoop
   make release
   ```

   `make release` (`scripts/release.sh`) refuses to run unless:
   - the tree is clean
   - HEAD has a `v*` tag that origin has at the same commit
   - the changelog section is dated
   - the tokens are set
   - the signing identity and the notary profile work

   It then builds a preview: signed, not notarized, nothing published. It
   prints every asset and repository the release will touch and waits for
   `y`. Then it runs `goreleaser release --clean` with notarization on. The
   notary service usually answers within a few minutes.

6. Do the checks under Verification on the published assets.

**If it fails part-way:** fix the cause and run `make release` again on the
same tag. `release.replace_existing_artifacts` lets assets be uploaded again.
To see why notarization was rejected:
`xcrun notarytool log <submission-id> --keychain-profile keepalive-notary`.

## Signing and notarization

`scripts/macos-sign.sh` runs on the universal binary as the GoReleaser
`universal_binaries` post hook.

- It signs with `codesign --force --options runtime --timestamp --identifier
  io.github.stigoleg.keepalive`, using the Developer ID above. The binary
  needs no entitlements under the hardened runtime: IOKit assertions,
  CoreGraphics events and the `responsibility_get_pid_responsible_for_pid`
  lookup all work.
- macOS ties the Accessibility grant for `--active` to the identifier and
  the team. Never change either, or every user has to grant the permission
  again.
- With `KEEPALIVE_NOTARIZE=1`, it zips the binary, submits it with
  `notarytool submit --wait`, and fails unless the status is `Accepted`.
  A bare command-line binary cannot be stapled. Gatekeeper checks the ticket
  online the first time a downloaded (quarantined) copy runs.
- The binary requires macOS 13 or later (`MACOSX_DEPLOYMENT_TARGET`).

| Variable | Effect |
|---|---|
| `KEEPALIVE_SIGN=0` | skip signing (CI, machines without the identity) |
| `KEEPALIVE_SIGN_IDENTITY` | another codesign identity |
| `KEEPALIVE_NOTARIZE=1` | notarize (set by `make release`; `make snapshot` forces 0) |
| `KEEPALIVE_NOTARY_PROFILE` | notarytool keychain profile, default `keepalive-notary` |
| `PUBLISH_PACKAGE_MANAGERS`, `GH_PAT` | push the Homebrew cask and the Scoop manifest |

## Verification

After `make snapshot`, or on the assets of a published release:

```sh
ls dist/
tar -tzf dist/keepalive_*_darwin_universal.tar.gz
b=dist/darwin_darwin_all/keepalive
lipo -archs "$b"                       # x86_64 arm64
codesign -dv --verbose=4 "$b"          # Developer ID, io.github.stigoleg.keepalive,
                                       # flags=0x10000(runtime), Timestamp=…
codesign --verify --strict --verbose=2 "$b"
"$b" doctor
```

After a release, download the macOS archive from GitHub so that it is
quarantined like a user's copy. Then run
`spctl --assess --type execute --verbose=2 keepalive` and `./keepalive
version`. If spctl only answers that the code "does not seem to be an app",
rely on `codesign -dv` and on the `Accepted` entry in `xcrun notarytool
history --keychain-profile keepalive-notary`.

Linux packages, in Docker (use the `amd64` package on an Intel host):

```sh
docker run --rm -v "$PWD/dist":/dist:ro ubuntu:24.04 sh -c '
  apt-get update -qq && apt-get install -y -qq /dist/keepalive_*_linux_arm64.deb &&
  keepalive version && cat /usr/lib/udev/rules.d/60-keepalive-uinput.rules'
```

**Teams presence (macOS).** This checks that `--active` keeps Microsoft
Teams Available in practice:

1. Note the time, start `keepalive -a`, and leave the Mac unlocked and
   untouched for at least 20 minutes.
2. Run the script with the start and end of that window:

   ```sh
   scripts/verify-teams-presence.py 2026-10-04T12:27 2026-10-04T12:50
   ```

   It reads the Teams logs and lists presence changes. Exit status 0 means
   Teams never reported you Away while the screen was unlocked.

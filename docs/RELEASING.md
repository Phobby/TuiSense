# Releasing TuiSense

This page is for maintainers. It explains how release artifacts are built,
signed and published, and what must be set up once before the first release.

## How trust works

TuiSense never trusts a download because of where it came from alone.

1. CI builds every archive on a native runner and smoke-tests `tuisense --version`.
2. `tools/release sums` writes `SHA256SUMS` listing every archive.
3. `SHA256SUMS` is signed with the **release key** using OpenSSH
   (`ssh-keygen -Y sign`, namespace `tuisense-release`), producing `SHA256SUMS.sig`.
4. Before publishing, CI verifies that signature against the public key that is
   **pinned in the repository** (`internal/update/trusted_keys.txt`).
5. The installers and the built-in updater verify the same signature against
   the key compiled or embedded into them, then check the archive's SHA-256
   against the signed list. Only then is anything executed.

A checksum file alone is not enough: an attacker who can replace an archive can
also replace an unsigned checksum list. The signature ties the list to a key
that only the maintainer (and the CI secret) holds.

GitHub build provenance attestations are also generated for every archive
(`gh attestation verify <file> -R Phobby/TuiSense`), as an additional,
optional check.

## One-time setup

Status: the release key has been generated and its public key is pinned
(fingerprint `SHA256:tWDwavuCx2W7m0gvo9RVwWI33wPw8hG5aYOUuORjRBc`). Steps 1 and
3 are done; steps 2 (the CI secret) and 4 (public repository) are made in
GitHub's settings. Without a pinned key the installers refuse to install and the
updater only checks for new versions.

### 1. Generate the release key

On a trusted machine:

```sh
ssh-keygen -t ed25519 -N "" -C tuisense-release -f tuisense_release_key
```

CI cannot answer a passphrase prompt, so the key has none; keep the private
key file offline (for example in a password manager) and never commit it.

### 2. Store the private key as a GitHub secret

Repository → Settings → Secrets and variables → Actions → New repository secret:

- Name: `TUISENSE_RELEASE_SIGNING_KEY`
- Value: the full contents of `tuisense_release_key` (the private key)

The release workflow fails instead of publishing an unsigned release when this
secret is missing.

### 3. Pin the public key in three places

Copy the single line from `tuisense_release_key.pub` (`ssh-ed25519 AAAA… tuisense-release`) into:

| File | Where |
| --- | --- |
| `internal/update/trusted_keys.txt` | as a new line (used by the built-in updater and CI verification) |
| `scripts/install.sh` | inside `TRUSTED_KEYS="…"` |
| `scripts/install.ps1` | inside `$TrustedKeys = @('…')` |

`go test ./internal/installtest` (`TestPinnedKeysAreInSync`) fails if the three
copies differ. Commit and push this change before tagging.

### 4. Make the repository public

The installers download `scripts/install.*` from `raw.githubusercontent.com` and
query the public GitHub Releases API without credentials. Neither works while
the repository is private.

## Publishing a release

1. Update `VERSION` (semantic versioning, e.g. `0.9.0`; prereleases use `0.9.0-rc.1`).
2. Run `make test` and commit.
3. Tag and push:

   ```sh
   git tag v$(cat VERSION)
   git push origin v$(cat VERSION)
   ```

4. The `Release` workflow (`.github/workflows/release.yml`) runs the tests,
   builds and smoke-tests six archives, signs and verifies the checksum list,
   attests provenance and creates the GitHub release with generated notes.
   Tags containing `-` are published as prereleases, which stable-channel users
   never receive.

### Artifacts

The ARM64 Linux and Windows runners are free only for **public** repositories;
in a private repository those jobs never start. `macos-15-intel` is GitHub's last
Intel macOS runner (supported until about August 2027); drop the Intel macOS
build when it is retired.

| Platform | Runner | Archive |
| --- | --- | --- |
| Linux x86-64 | `ubuntu-latest` | `tuisense_<v>_linux_amd64.tar.gz` |
| Linux ARM64 | `ubuntu-24.04-arm` | `tuisense_<v>_linux_arm64.tar.gz` |
| macOS Intel | `macos-15-intel` | `tuisense_<v>_darwin_amd64.tar.gz` |
| macOS Apple Silicon | `macos-15` | `tuisense_<v>_darwin_arm64.tar.gz` |
| Windows x86-64 | `windows-latest` | `tuisense_<v>_windows_amd64.zip` |
| Windows ARM64 | `windows-11-arm` | `tuisense_<v>_windows_arm64.zip` |

Each binary is a static Go build (`CGO_ENABLED=0`); no runtime libraries are
bundled or required.

macOS binaries are not signed with an Apple Developer ID or notarized. Files
downloaded with `curl` (the installer) are not quarantined, so they run
normally; an archive downloaded with a web browser is quarantined and macOS
will ask for confirmation the first time (see the README's macOS notes).

## Local checks

```sh
go test ./internal/update ./internal/installtest   # updater + installer, against a local mock release server
go run ./tools/release package -version 0.0.0 -os linux -arch amd64 -bin bin/tuisense -out /tmp/dist
go run ./tools/release sums -out /tmp/dist
```

## Rotating or revoking the key

Add the new public key next to the old one in all three places, release once
signed with the new key, then remove the old key in a following release. If the
private key leaks, remove its public key immediately and publish a release signed
with a new key; installs that still pin only the old key must be reinstalled
with the updated installer.

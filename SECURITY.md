# Security Policy

## Supported versions

Only the latest release receives security fixes. Update with `tuisense update`
or by re-running the installer.

## Reporting a vulnerability

Please **do not** open a public issue for security problems. Report them
privately through GitHub:
[Security → Report a vulnerability](https://github.com/Phobby/TuiSense/security/advisories/new).

Include what you found, how to reproduce it, and which version and operating
system you used. You will get an acknowledgement as soon as possible, and a fix
or mitigation will be coordinated with you before details are made public.

## Areas of particular interest

- The self-updater and the installers (`internal/update`, `scripts/install.sh`,
  `scripts/install.ps1`): anything that lets an unsigned or tampered binary be
  installed or run.
- Device access: anything that sends unexpected output reports to a controller
  or touches devices other than game controllers.
- Reports and clipboard: anything that leaks identifiers (serial numbers, MAC
  addresses) without the user opting in.

## Release signing

Release checksum lists are signed with an Ed25519 key (OpenSSH `SSHSIG`,
namespace `tuisense-release`). The public key is pinned in
[`internal/update/trusted_keys.txt`](internal/update/trusted_keys.txt):

```
SHA256:tWDwavuCx2W7m0gvo9RVwWI33wPw8hG5aYOUuORjRBc
```

See [docs/RELEASING.md](docs/RELEASING.md) for how releases are verified.

TuiSense never installs or flashes controller firmware.

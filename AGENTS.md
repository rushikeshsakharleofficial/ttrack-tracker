# AGENTS.md — ttrack

## Quick Facts
- Go module `ttrack`; CI uses Go 1.25, builds must be static (`CGO_ENABLED=0`).
- Two binaries: `cmd/ttrack` (CLI) and `cmd/ttrackd` (daemon).
- Storage locations:
  - Local: `$TTRACK_DIR` (default `~/.local/share/ttrack`).
  - Central (encrypted): `$TTRACK_CENTRAL_DIR` (`/var/lib/ttrack`).
  - Encryption key at `<central>/.ttrack.key` (immutable via `chattr +i`).

## Common Commands
- `make fmt` → `go fmt ./...`
- `make vet` → `go vet ./...`
- `make test` → `go test ./...`
- `make build` → static build (`CGO_ENABLED=0 go build -trimpath`) → `bin/ttrack`, `bin/ttrackd`
- `make VERSION=… packages` → `nfpm` builds RPM & DEB in `release/`
- `make install` → installs CLI to `/usr/bin`, daemon to `/usr/libexec/ttrackd`, systemd unit, completions, man page.

## CI / Jenkins Pipeline (Jenkinsfile)
- Stages: Checkout → Release Version → Format → Vet → Test → Build → Package → **Publish GitHub Release** → SonarQube → Deploy to Jump Server.
- `options { disableConcurrentBuilds() }` prevents executor dead‑locks.
- **Release Version** stage:
  * Uses `git tag --points-at HEAD` for a `vX.Y.Z` tag; otherwise increments the patch of the latest tag.
  * Sets `env.RELEASE_VERSION` for later stages.
* Updates version strings in `Makefile`, `man/ttrack.1` (both quoted and unquoted forms), and `README.md` via robust `sed` commands.
- **Publish GitHub Release** stage:
  * Installs `gh` (`go install github.com/cli/cli/v2/cmd/gh@v2.87.3`) if missing.
  * Uses credential `github-release-token`.
  * Creates a GitHub release `v${RELEASE_VERSION}` (if not present) and uploads `release/*.rpm`, `release/*.deb`, binary, and `SHA256SUMS`.
- **Deploy to Jump Server** stage:
  * Copies the latest `.deb` via `scp`.
  * Installs with `sudo -n dpkg -i` (non‑interactive) to avoid hanging the pipeline.
  * Uses `jump-server-key` credential.
- Environment for Go in Jenkins: `GOROOT`, `GOPATH`, `GOCACHE`, `CGO_ENABLED=0`.
- All stages archive relevant artifacts (`archiveArtifacts`).

## Gotchas
- Central casts are AES‑256‑GCM; read/write only via `store.OpenCast*` APIs.
- Daemon socket (`/run/ttrackd.sock`) is mode 0666; privacy enforced by file permissions, not socket.
- Tests that touch the store must set `TTRACK_DIR`, `TTRACK_CENTRAL_DIR`, and `TTRACKD_SOCK` to temporary locations.
- The daemon creates `.ttrack.key` as immutable; losing it makes encrypted recordings unreadable.
- `sudo -n` in the Deploy stage is required; a normal `sudo` would block the pipeline awaiting a password.

## Ansible Integration
- Callback plugin `scripts/ansible/ttrack.py` writes JSON Lines to `~/.local/share/ttrack/ansible/<runid>.ajsonl` (0600) and to central `/var/lib/ttrack/...`.
- Enable with:
  ```
  ANSIBLE_CALLBACK_PLUGINS=/usr/share/ttrack/ansible
  ANSIBLE_CALLBACKS_ENABLED=ttrack
  ```
- Packaging (nfpm) currently does **not** install the plugin; verify `nfpm.yaml` when adding plugin support.

## Packaging Details
- `nfpm.yaml` builds RPM and DEB containing CLI, daemon, systemd unit, man page, and completions.
- `make packages` expects `VERSION` (set by Release Version stage) and cleans old artefacts before building.
- SHA256 sums are generated in `release/SHA256SUMS` and uploaded to GitHub releases.

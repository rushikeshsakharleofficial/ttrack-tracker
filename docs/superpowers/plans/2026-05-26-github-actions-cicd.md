# GitHub Actions CI/CD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GitHub Actions CI (build/vet/test/format on every push & PR) and a tag-triggered Release pipeline that publishes the static binary plus RPM and DEB packages to a GitHub Release.

**Architecture:** Two workflows. `ci.yml` runs on push to `main` and on pull requests: it checks formatting, vets, tests, and builds the static binary. `release.yml` runs on `v*` tags: it builds the binary, packages RPM+DEB with the existing `nfpm.yaml`, generates checksums, and creates a GitHub Release with all artifacts attached. Workflows are validated locally with `actionlint` before pushing.

**Tech Stack:** GitHub Actions, `actions/checkout@v4`, `actions/setup-go@v5`, Go 1.25, `nfpm` (already used by `make packages`), `softprops/action-gh-release@v2`, `actionlint` for local validation.

**Repo facts (verified):**
- Remote: `https://github.com/rushikeshsakharleofficial/terminal-session-recorder.git`
- Module `ttrack`, main package `./cmd/ttrack`, `go 1.25.9`
- `make build` = `CGO_ENABLED=0 go build -trimpath -o bin/ttrack ./cmd/ttrack`
- `make packages` builds RPM+DEB via `nfpm.yaml`, reading version from `TTRACK_VERSION`
- No `.github/` directory exists yet

---

### Task 1: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create the CI workflow file**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  build-test:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true

      - name: Check formatting
        run: |
          unformatted="$(gofmt -l .)"
          if [ -n "$unformatted" ]; then
            echo "These files are not gofmt-clean:"
            echo "$unformatted"
            exit 1
          fi

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./... -count=1

      - name: Build (static)
        run: CGO_ENABLED=0 go build -trimpath -o bin/ttrack ./cmd/ttrack

      - name: Smoke test binary
        run: ./bin/ttrack --help
```

- [ ] **Step 2: Validate YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('ci.yml OK')"`
Expected: `ci.yml OK`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add build/vet/test/gofmt workflow"
```

---

### Task 2: Release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create the release workflow file**

Create `.github/workflows/release.yml`. The tag `v0.1.0` yields package version `0.1.0` via `${GITHUB_REF_NAME#v}`.

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true

      - name: Derive version from tag
        id: ver
        run: echo "version=${GITHUB_REF_NAME#v}" >> "$GITHUB_OUTPUT"

      - name: Build (static)
        run: CGO_ENABLED=0 go build -trimpath -o bin/ttrack ./cmd/ttrack

      - name: Install nfpm
        run: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

      - name: Build packages
        env:
          TTRACK_VERSION: ${{ steps.ver.outputs.version }}
        run: |
          mkdir -p release
          NFPM="$(go env GOPATH)/bin/nfpm"
          "$NFPM" pkg --config nfpm.yaml --packager rpm --target release/
          "$NFPM" pkg --config nfpm.yaml --packager deb --target release/
          cp bin/ttrack "release/ttrack-${TTRACK_VERSION}-linux-amd64"

      - name: Generate checksums
        run: cd release && sha256sum * > SHA256SUMS

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            release/*.rpm
            release/*.deb
            release/ttrack-*-linux-amd64
            release/SHA256SUMS
          generate_release_notes: true
```

- [ ] **Step 2: Validate YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('release.yml OK')"`
Expected: `release.yml OK`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add tag-triggered release workflow (binary + rpm + deb)"
```

---

### Task 3: Validate both workflows with actionlint

**Files:** none (validation only)

- [ ] **Step 1: Install actionlint**

Run: `go install github.com/rhysd/actionlint/cmd/actionlint@latest`
Expected: completes without error; binary at `$(go env GOPATH)/bin/actionlint`

- [ ] **Step 2: Lint both workflows**

Run: `"$(go env GOPATH)/bin/actionlint" .github/workflows/ci.yml .github/workflows/release.yml`
Expected: no output (exit 0). If `shellcheck` is absent, actionlint skips shell checks — that is acceptable.

- [ ] **Step 3: Fix any findings**

If actionlint reports issues, edit the workflow file(s) to resolve them, re-run Step 2 until clean. No commit if no changes; otherwise:

```bash
git add .github/workflows/
git commit -m "ci: fix actionlint findings"
```

---

### Task 4: CI status badge in README

**Files:**
- Modify: `README.md` (insert badge line directly under the `# ttrack` title, line 1)

- [ ] **Step 1: Add the badge**

Insert this line immediately after the `# ttrack` heading (becomes the new line 2), followed by a blank line:

```markdown
[![CI](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/actions/workflows/ci.yml/badge.svg)](https://github.com/rushikeshsakharleofficial/terminal-session-recorder/actions/workflows/ci.yml)
```

- [ ] **Step 2: Verify it renders as valid markdown (no broken link syntax)**

Run: `head -3 README.md`
Expected: title line, then the badge line containing `actions/workflows/ci.yml/badge.svg`.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add CI status badge"
```

---

### Task 5: Push and verify Actions run

**Files:** none

- [ ] **Step 1: Push to origin**

```bash
git push origin main
```
Expected: push succeeds.

- [ ] **Step 2: Watch the CI run**

Run: `gh run list --workflow=ci.yml --limit 1`
Then: `gh run watch "$(gh run list --workflow=ci.yml --limit 1 --json databaseId -q '.[0].databaseId')" --exit-status`
Expected: the run completes with conclusion `success`. If it fails, open logs with `gh run view --log-failed` and fix the underlying issue.

- [ ] **Step 3 (optional, confirm with user before running): Smoke-test the release pipeline**

> Destructive-ish: creates a public GitHub Release and a tag. Only do this with user approval.

```bash
git tag v0.1.0
git push origin v0.1.0
gh run watch "$(gh run list --workflow=release.yml --limit 1 --json databaseId -q '.[0].databaseId')" --exit-status
gh release view v0.1.0
```
Expected: release `v0.1.0` exists with `*.rpm`, `*.deb`, `ttrack-0.1.0-linux-amd64`, and `SHA256SUMS` attached.

---

## Self-Review

**Spec coverage:**
- CI (build/vet/test/format) → Task 1 ✓
- Release (tag → binary + RPM + DEB on GitHub Release) → Task 2 ✓
- Validation → Task 3 ✓
- Badge → Task 4 ✓
- Push + verify → Task 5 ✓

**Placeholder scan:** No TBD/TODO; every workflow file is shown in full; commands have expected output.

**Type/name consistency:** `TTRACK_VERSION` env matches `nfpm.yaml`'s `${TTRACK_VERSION}` and `make`'s variable. `GITHUB_REF_NAME` strip-`v` produces the bare version nfpm expects. Workflow filenames referenced in the badge (`ci.yml`) and in `gh run list --workflow=` match the created files.

**Notes:**
- `release/` and `/bin/` are gitignored, so artifacts are not committed; the release job rebuilds them.
- `permissions: contents: write` on release is required for `action-gh-release` to upload assets.

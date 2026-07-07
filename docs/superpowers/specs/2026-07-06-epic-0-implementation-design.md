# Design: Epic 0 Implementation — Repository Foundations & Project Hygiene (actual files)

- **Issue:** N/A — Epic 0 implementation
- **Epic:** 0 — Repository Foundations & Project Hygiene
- **Date:** 2026-07-06
- **Status:** Approved
- **Branch:** `docs/epic-0-repo-foundations` → PR #9 (stacked on `refactor/router-pr-submodel`)
- **Implements roadmap entry:** `docs/superpowers/specs/2026-07-06-epic-0-repo-foundations-design.md`

## Context

`lazygh` is a Go 1.25.1 terminal-UI app (module `github.com/ChristopherBilg/lazygh`), 7
source files, currently `gofmt`- and `go vet`-clean. The only repo-hygiene file present is
`.github/dependabot.yml` (Go modules only, weekly, no grouping). There is no `LICENSE`,
`.gitignore`, CI workflow, lint config, `Makefile`, `.editorconfig`, or any community-health
/ template files.

The Epic 0 roadmap entry (already merged into `README.md` on this branch) *describes* this
foundational work. This spec covers **actually building it** — the real files — on the same
branch/PR.

## Goals

- Create the foundational files Epic 0 names, on branch `docs/epic-0-repo-foundations`.
- **CI lands green:** `go build`, `go vet`, `gofmt` check, `go test`, and `golangci-lint`
  (standard set) all pass — fixing existing code where the linters flag it.
- **No email published anywhere** — security and conduct reporting route through GitHub.

## Non-goals / Out of scope

- Repo/GitHub settings that cannot be done via git — enabling Private Vulnerability
  Reporting and adding branch-protection required checks. These are documented as manual
  follow-ups, not implemented here.
- YAML issue *forms* (using classic Markdown templates instead).
- A Go version matrix in CI (single version from `go.mod`).
- Any change to application behavior. Code edits are lint-driven quality fixes only.
- The "onboarding extras" (`SUPPORT.md`, `FUNDING.yml`, `CHANGELOG`) — excluded in the
  roadmap entry, still excluded here.

## Key decisions (all user-confirmed)

1. **Lint:** golangci-lint **v2**, `default: standard` linters (`errcheck`, `govet`,
   `ineffassign`, `staticcheck`, `unused`) + `gofmt`/`goimports` formatters. Fix existing
   code so the gate is green.
2. **Contacts — no email.** `SECURITY.md` → GitHub Private Vulnerability Reporting.
   `CODE_OF_CONDUCT.md` → reports to the maintainer via GitHub profile
   (`https://github.com/ChristopherBilg`). (Accepted trade-off: GitHub has no private DM, so
   the CoC channel is weaker than a private inbox.)
3. **License:** MIT, `Copyright (c) 2026 Christopher R. Bilger`.
4. **CODEOWNERS:** `* @ChristopherBilg`.
5. **CI:** `.github/workflows/ci.yml`, triggers on `pull_request` + `push` to `main`; two
   parallel jobs (`test`, `lint`); third-party actions **pinned to commit SHAs** (version in
   a trailing comment); `setup-go` with `go-version-file: go.mod` + module cache. The `test`
   job invokes `Makefile` targets (`build`/`vet`/`test`) so local and CI cannot drift; the
   `lint` job uses the official `golangci-lint-action` (recommended — handles install +
   caching) against the same `.golangci.yml`.
6. **Issue templates:** classic Markdown; blank issues disabled.
7. **Dependabot:** add a `github-actions` ecosystem (weekly) and `groups` (minor/patch) for
   both ecosystems.

## File-by-file design

Concrete content is given for the technical files (where exactness matters); prose files are
described by source + section outline. Action SHAs and the exact golangci-lint version are
resolved at implementation time (via `gh api` / the golangci-lint release), so the plan
contains no placeholders.

### A — CI & lint

**`.github/workflows/ci.yml`** (near-final; SHAs filled in during implementation):

```yaml
name: CI

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<sha> # vX.Y.Z
      - uses: actions/setup-go@<sha> # vX.Y.Z
        with:
          go-version-file: go.mod
          cache: true
      - run: make build
      - run: make vet
      - run: make test

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<sha> # vX.Y.Z
      - uses: actions/setup-go@<sha> # vX.Y.Z
        with:
          go-version-file: go.mod
          cache: true
      - uses: golangci/golangci-lint-action@<sha> # vX.Y.Z
        with:
          version: v2.<x>   # latest v2
```

Formatting is enforced by the `lint` job (golangci-lint's `gofmt`/`goimports` formatters),
satisfying the roadmap's "enforce gofmt/goimports" item; `go vet` also runs explicitly in
`test` for clarity.

**`.golangci.yml`** (v2 schema):

```yaml
version: "2"
linters:
  default: standard   # errcheck, govet, ineffassign, staticcheck, unused
formatters:
  enable:
    - gofmt
    - goimports
```

### B — Community health

- **`LICENSE`** — MIT license, verbatim, `Copyright (c) 2026 Christopher R. Bilger`.
- **`CONTRIBUTING.md`** — sections: *Getting set up* (Go 1.25+, `gh auth login`), *Build &
  run* (`make build` / `make run`), *Test & lint* (`make test` / `make lint`; how to install
  golangci-lint v2), *Coding style* (gofmt/goimports enforced by CI; `make fmt` to fix),
  *Branches & PRs* (branch naming, small focused PRs, fill the PR template, link issues),
  and a link to the Code of Conduct.
- **`CODE_OF_CONDUCT.md`** — Contributor Covenant **v2.1** verbatim, with the enforcement
  contact set to "report to the project maintainer via GitHub —
  https://github.com/ChristopherBilg" (no email).
- **`SECURITY.md`** — *Supported Versions* (pre-1.0: fixes land on `main`) and *Reporting a
  Vulnerability* (use GitHub → **Security → Report a vulnerability** / Private Vulnerability
  Reporting; do not open public issues for security reports). No email.

### C — Templates & routing

- **`.github/ISSUE_TEMPLATE/bug_report.md`** — front matter (`name`, `about`,
  `labels: bug`); sections: Description, Steps to reproduce, Expected, Actual, Environment
  (OS, terminal, `lazygh` version, `gh` version).
- **`.github/ISSUE_TEMPLATE/feature_request.md`** — front matter (`labels: enhancement`);
  sections: Problem/motivation, Proposed solution, Alternatives, Additional context.
- **`.github/ISSUE_TEMPLATE/config.yml`** — `blank_issues_enabled: false`.
- **`.github/PULL_REQUEST_TEMPLATE.md`** — Summary, Related issue (`Closes #`), Changes,
  Checklist (tests pass locally, `make lint` clean, docs updated if needed).
- **`.github/CODEOWNERS`** — `* @ChristopherBilg`.

### D — Dev tooling & hygiene

- **`.gitignore`**:

```gitignore
# Binaries / build output
/bin/
/lazygh
*.exe
*.test
*.out

# Coverage
*.cover
coverage.*

# Go workspace
go.work
go.work.sum

# Editor / OS
.idea/
.vscode/
*.swp
.DS_Store

# Local application state / logs (see Epic 1 logging)
.local/
```

- **`.editorconfig`**:

```editorconfig
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = space
indent_size = 2

[*.go]
indent_style = tab

[Makefile]
indent_style = tab

[*.md]
trim_trailing_whitespace = false
```

- **`Makefile`** (tabs for recipes):

```makefile
.PHONY: build run test vet fmt lint tidy clean
BINARY := bin/lazygh

build:
	go build -o $(BINARY) ./cmd/lazygh

run:
	go run ./cmd/lazygh

test:
	go test ./...

vet:
	go vet ./...

fmt:
	golangci-lint fmt

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin
```

(`make fmt` uses golangci-lint v2's `fmt` subcommand so formatting uses the same tool/config
as the CI lint job; CONTRIBUTING documents installing golangci-lint.)

- **`.github/dependabot.yml`** (modified):

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    groups:
      go-minor-patch:
        update-types: ["minor", "patch"]
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    groups:
      actions-minor-patch:
        update-types: ["minor", "patch"]
```

### E — Code fixes (lint conformance)

After the `.golangci.yml` exists, run golangci-lint locally and fix each finding in the 7 Go
files (`cmd/lazygh/main.go`, `internal/github/client.go`, `internal/tui/router.go`,
`internal/tui/pr/pr.go`, `internal/tui/repolist/repolist.go`, `internal/tui/screen/screen.go`,
`internal/tui/styles/styles.go`). Expected findings are small and mechanical (e.g. unchecked
errors from `errcheck`). No behavior changes; existing tests must still pass. The exact set
is enumerated in the plan after the first lint run.

## Implementation & verification approach

- All work on `docs/epic-0-repo-foundations` (PR #9). Subagent-driven, roughly one task per
  group (A–E), with spec + quality review per task and a final review, then push.
- Toolchain: `go install` golangci-lint v2 locally to run the gate; resolve action commit
  SHAs via `gh api`.
- **Verification (must all pass before commits land):** `go build ./...`, `go vet ./...`,
  `gofmt -l .` (empty), `go test ./...`, `golangci-lint run` (clean); `actionlint` on the
  workflow if available; a YAML parse of every new `.yml`; `make build`/`test`/`lint` run.

## Manual follow-ups (cannot be done via git)

- Enable **Private Vulnerability Reporting** (repo Settings → Code security) so `SECURITY.md`
  works.
- Optionally add the `test` and `lint` checks to branch-protection required status checks on
  `main`.

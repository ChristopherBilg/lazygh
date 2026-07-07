# Epic 0 Implementation Plan — Repository Foundations & Project Hygiene

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create Epic 0's foundational files on branch `docs/epic-0-repo-foundations` (PR #9) — CI + lint gate, community-health files, GitHub templates/routing, developer tooling, and a Dependabot extension — with CI landing green.

**Architecture:** Config/docs scaffolding plus five small lint-driven code fixes. Five tasks grouped by concern; the CI task depends on the `Makefile` (Task 1) and the green lint gate (Task 2). No application behavior changes.

**Tech Stack:** Go 1.25, GitHub Actions, golangci-lint v2, Make, EditorConfig, Dependabot.

**Spec:** `docs/superpowers/specs/2026-07-06-epic-0-implementation-design.md`

**Resolved pinned versions** (verified via GitHub API on 2026-07-06):
- `actions/checkout` v7.0.0 → `9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0`
- `actions/setup-go` v6.5.0 → `924ae3a1cded613372ab5595356fb5720e22ba16`
- `golangci/golangci-lint-action` v9.3.0 → `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a`
- `golangci-lint` v2.12.2

**Note on TDD:** This is mostly config/docs (no unit tests to write first); verification is command/structural. The one code-touching task (Task 2) is guarded by the existing test suite (`go test ./...`) plus `golangci-lint run`. The 5 fixes in Task 2, the `.golangci.yml` schema, and the whole toolchain were validated locally during planning → `0 issues` + all tests pass.

**Execution:** In-place on the current branch `docs/epic-0-repo-foundations` (no worktree); each task commits, and PR #9 updates on push. Stage only the files each task creates (never `git add -A`).

**Toolchain note:** `go install` places `golangci-lint`/`actionlint` in `$(go env GOPATH)/bin`, which is not on PATH here, and a shell `export` does not persist across separate command invocations. In every command that runs `golangci-lint`, `actionlint`, or `make lint`/`make fmt`, prepend `export PATH="$(go env GOPATH)/bin:$PATH"; ` (or add that dir to your shell profile once) so the binaries resolve.

---

### Task 1: Developer tooling & hygiene (`.gitignore`, `.editorconfig`, `Makefile`)

**Goal:** The repo gains a Go `.gitignore`, an `.editorconfig`, and a `Makefile` whose `build`/`vet`/`test` targets work (CI will call these).

**Files:**
- Create: `.gitignore`
- Create: `.editorconfig`
- Create: `Makefile`

**Acceptance Criteria:**
- [ ] All three files exist.
- [ ] `make build` produces `bin/lazygh`; `make vet` and `make test` succeed.
- [ ] `bin/` is git-ignored (`git check-ignore bin/lazygh` prints the path).

**Verify:** `make build && make vet && make test` → build succeeds, vet silent, tests `ok`; `git check-ignore bin/lazygh` → `bin/lazygh`.

**Steps:**

- [ ] **Step 1: Create `.gitignore`**

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

- [ ] **Step 2: Create `.editorconfig`**

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

- [ ] **Step 3: Create `Makefile`** (recipes MUST be indented with a real tab)

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

- [ ] **Step 4: Verify**

Run: `make build && make vet && make test`
Expected: `bin/lazygh` created; vet prints nothing; tests print `ok` for the three test packages.
Run: `git check-ignore bin/lazygh`
Expected: `bin/lazygh`

- [ ] **Step 5: Commit**

```bash
git add .gitignore .editorconfig Makefile
git commit -m "chore: add .gitignore, .editorconfig, and Makefile

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Lint gate — `.golangci.yml` + fix 5 staticcheck findings

**Goal:** Establish the golangci-lint v2 gate and make the existing code pass it (`golangci-lint run` → `0 issues`), with tests still green.

**Files:**
- Create: `.golangci.yml`
- Modify: `internal/tui/pr/pr.go` (3 lines: 124, 134, 223)
- Modify: `internal/tui/repolist/repolist.go` (2 lines: 111, 115)

**Acceptance Criteria:**
- [ ] `.golangci.yml` uses the v2 schema, standard linters + gofmt/goimports formatters.
- [ ] `golangci-lint run` reports `0 issues`.
- [ ] `go test ./...` passes (behavior unchanged).

**Verify:** install golangci-lint v2.12.2, then `golangci-lint run` → `0 issues` (exit 0); `go test ./...` → all `ok`.

**Steps:**

- [ ] **Step 1: Install golangci-lint v2.12.2** (not preinstalled)

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
export PATH="$(go env GOPATH)/bin:$PATH"
golangci-lint version   # -> 2.12.2
```

- [ ] **Step 2: Create `.golangci.yml`**

```yaml
version: "2"
linters:
  default: standard
formatters:
  enable:
    - gofmt
    - goimports
```

- [ ] **Step 3: Confirm the failures the gate reports** (baseline before fixing)

Run: `golangci-lint run`
Expected: `5 issues` — SA1019 at `internal/tui/pr/pr.go:124` and `:134`; QF1012 at `internal/tui/pr/pr.go:223`, `internal/tui/repolist/repolist.go:111`, `:115`.

- [ ] **Step 4: Fix SA1019 (deprecated viewport scroll methods) in `internal/tui/pr/pr.go`**

Replace (line ~124):
```go
				m.viewport.LineUp(1)
```
with:
```go
				m.viewport.ScrollUp(1)
```

Replace (line ~134):
```go
				m.viewport.LineDown(1)
```
with:
```go
				m.viewport.ScrollDown(1)
```

- [ ] **Step 5: Fix QF1012 (WriteString+Sprintf → Fprintf) in `internal/tui/pr/pr.go`**

Replace (line ~223):
```go
		listStr.WriteString(fmt.Sprintf("%s%s\n", cursorStr, title))
```
with:
```go
		fmt.Fprintf(&listStr, "%s%s\n", cursorStr, title)
```
(`listStr` is a `strings.Builder` value declared at line ~210, so `&listStr` satisfies `io.Writer`; `fmt` is already imported.)

- [ ] **Step 6: Fix QF1012 in `internal/tui/repolist/repolist.go`**

Replace (line ~111):
```go
		s.WriteString(fmt.Sprintf("%s%s\n", cursor, repoName))
```
with:
```go
		fmt.Fprintf(&s, "%s%s\n", cursor, repoName)
```

Replace (line ~115):
```go
		s.WriteString(fmt.Sprintf("\n  ...and %d more.\n", len(m.repos)-maxVisible))
```
with:
```go
		fmt.Fprintf(&s, "\n  ...and %d more.\n", len(m.repos)-maxVisible)
```
(`s` is a `strings.Builder` value declared at line ~94; `fmt` and `strings` remain used, so imports are unchanged.)

- [ ] **Step 7: Verify clean gate + green tests**

Run: `golangci-lint run`
Expected: `0 issues` (exit 0).
Run: `gofmt -l internal/tui/pr/pr.go internal/tui/repolist/repolist.go`
Expected: (empty)
Run: `go build ./... && go test ./...`
Expected: build OK; all test packages `ok`.

- [ ] **Step 8: Commit**

```bash
git add .golangci.yml internal/tui/pr/pr.go internal/tui/repolist/repolist.go
git commit -m "ci: add golangci-lint v2 config and satisfy the standard linters

Adds .golangci.yml (v2 schema, standard linters + gofmt/goimports) and
fixes the 5 staticcheck findings it surfaces: replace deprecated
viewport.LineUp/LineDown with ScrollUp/ScrollDown, and replace
WriteString(fmt.Sprintf(...)) with fmt.Fprintf(...). No behavior change.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: CI workflow + Dependabot extension  *(blocked by Tasks 1 and 2)*

**Goal:** Add `.github/workflows/ci.yml` (lint + test on PRs / push to `main`) with SHA-pinned actions, and extend Dependabot to cover GitHub Actions with grouped updates.

**Files:**
- Create: `.github/workflows/ci.yml`
- Modify: `.github/dependabot.yml`

**Acceptance Criteria:**
- [ ] `ci.yml` triggers on `pull_request` and `push` to `main`, with `test` and `lint` jobs.
- [ ] All third-party actions are pinned to the commit SHAs listed above (with version comments).
- [ ] `dependabot.yml` has both `gomod` and `github-actions` ecosystems, each with a minor/patch group.
- [ ] `actionlint` passes; the CI-equivalent commands pass locally.

**Verify:** `actionlint` → no errors; `make build && make vet && make test && golangci-lint run` → all pass; `dependabot.yml` is valid YAML.

**Steps:**

- [ ] **Step 1: Create `.github/workflows/ci.yml`**

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
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0
        with:
          go-version-file: go.mod
          cache: true
      - run: make build
      - run: make vet
      - run: make test

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0
        with:
          go-version-file: go.mod
          cache: true
      - uses: golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9.3.0
        with:
          version: v2.12.2
```

- [ ] **Step 2: Replace `.github/dependabot.yml`** with:

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

- [ ] **Step 3: Validate the workflow with actionlint**

```bash
go install github.com/rhysd/actionlint/cmd/actionlint@latest
"$(go env GOPATH)/bin/actionlint"
```
Expected: no output (exit 0).

- [ ] **Step 4: Run the CI-equivalent locally**

Run: `make build && make vet && make test && golangci-lint run`
Expected: build OK; vet silent; tests `ok`; `0 issues`.
(Optional YAML sanity for dependabot: `python3 -c "import yaml; yaml.safe_load(open('.github/dependabot.yml')); print('ok')"` if PyYAML is present — otherwise GitHub validates the schema on push.)

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml .github/dependabot.yml
git commit -m "ci: add GitHub Actions CI workflow and extend Dependabot

Add ci.yml running test (build/vet/test via make) and lint
(golangci-lint) on pull requests and pushes to main, with actions pinned
to commit SHAs. Extend Dependabot to also update the github-actions
ecosystem and group minor/patch bumps for both ecosystems.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Community-health files (`LICENSE`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`)

**Goal:** Add the standard community-health files, with all reporting routed through GitHub (no email anywhere).

**Files:**
- Create: `LICENSE`
- Create: `CONTRIBUTING.md`
- Create: `CODE_OF_CONDUCT.md`
- Create: `SECURITY.md`

**Acceptance Criteria:**
- [ ] `LICENSE` is MIT with `Copyright (c) 2026 Christopher R. Bilger`.
- [ ] No email address appears in any of the four files.
- [ ] `SECURITY.md` routes to GitHub Private Vulnerability Reporting; `CODE_OF_CONDUCT.md` routes CoC reports to the maintainer via GitHub.

**Verify:** `grep -rIE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' LICENSE CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md` → no matches (exit 1); files present.

**Steps:**

- [ ] **Step 1: Create `LICENSE`** (MIT)

```text
MIT License

Copyright (c) 2026 Christopher R. Bilger

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Create `CONTRIBUTING.md`**

```markdown
# Contributing to lazygh

Thanks for your interest in improving `lazygh`! This document covers how to get
set up and the conventions we follow.

## Getting set up

- Install **Go 1.25+** (the toolchain version is pinned in `go.mod`).
- Authenticate the GitHub CLI: `gh auth login` (lazygh uses your `gh` credentials).
- Install the linter (used by CI): `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`.

## Build & run

- `make build` — compile to `bin/lazygh`.
- `make run` — run the app.

## Test & lint

- `make test` — run the unit tests.
- `make vet` — run `go vet`.
- `make lint` — run golangci-lint.
- `make fmt` — auto-format (gofmt + goimports via golangci-lint).

CI runs the same `test` and `lint` steps on every pull request; please make sure
both pass locally first.

## Coding style

- Code must be `gofmt`/`goimports`-clean and pass the linters in `.golangci.yml`.
- Keep changes focused; match the surrounding style and the Elm-architecture
  structure documented under `docs/`.

## Branches & pull requests

- Branch from `main` with a descriptive name (e.g. `feat/...`, `fix/...`, `docs/...`).
- Keep PRs small and focused; fill out the pull-request template and link any
  related issue (e.g. `Closes #123`).

## Code of Conduct

This project follows its [Code of Conduct](CODE_OF_CONDUCT.md). By participating,
you agree to uphold it.
```

- [ ] **Step 3: Create `CODE_OF_CONDUCT.md`** (Contributor Covenant v2.1)

Fetch the canonical Contributor Covenant v2.1 text from
`https://www.contributor-covenant.org/version/2/1/code_of_conduct.md` and save it as
`CODE_OF_CONDUCT.md`. In the **Enforcement** section, replace the placeholder
`[INSERT CONTACT METHOD]` with exactly:

```
the project maintainer via GitHub at https://github.com/ChristopherBilg
```

Confirm the saved file contains no email address and no remaining `[INSERT CONTACT METHOD]`
placeholder. (If fetching is unavailable, use the Contributor Covenant v2.1 markdown text
with that same enforcement contact.)

- [ ] **Step 4: Create `SECURITY.md`**

```markdown
# Security Policy

## Supported Versions

`lazygh` is pre-1.0 and under active development. Security fixes are applied to
the latest `main`; there are no long-term support branches yet.

## Reporting a Vulnerability

Please **do not** open a public issue for security reports.

Instead, use GitHub's private vulnerability reporting for this repository:
open the **Security** tab and choose **"Report a vulnerability"** (Private
Vulnerability Reporting). This keeps the report confidential until a fix is
available.

We'll acknowledge the report, investigate, and coordinate a fix and disclosure
timeline with you.
```

- [ ] **Step 5: Verify no email present**

Run: `grep -rIE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' LICENSE CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md`
Expected: no output (exit status 1).

- [ ] **Step 6: Commit**

```bash
git add LICENSE CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md
git commit -m "docs: add LICENSE (MIT), CONTRIBUTING, CODE_OF_CONDUCT, SECURITY

Community-health files. MIT license (2026 Christopher R. Bilger).
Security reports route to GitHub Private Vulnerability Reporting and CoC
reports to the maintainer via GitHub; no email addresses are published.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: GitHub templates & review routing

**Goal:** Add issue templates (bug + feature) with blank issues disabled, a pull-request template, and CODEOWNERS.

**Files:**
- Create: `.github/ISSUE_TEMPLATE/bug_report.md`
- Create: `.github/ISSUE_TEMPLATE/feature_request.md`
- Create: `.github/ISSUE_TEMPLATE/config.yml`
- Create: `.github/PULL_REQUEST_TEMPLATE.md`
- Create: `.github/CODEOWNERS`

**Acceptance Criteria:**
- [ ] Both issue templates and a `config.yml` (`blank_issues_enabled: false`) exist.
- [ ] A PR template with a checklist exists.
- [ ] `CODEOWNERS` assigns everything to `@ChristopherBilg`.

**Verify:** files exist; `config.yml` is valid YAML; `grep -q '^\* @ChristopherBilg$' .github/CODEOWNERS`.

**Steps:**

- [ ] **Step 1: Create `.github/ISSUE_TEMPLATE/bug_report.md`**

```markdown
---
name: Bug report
about: Report a problem with lazygh
title: "[Bug]: "
labels: [bug]
---

## Description

A clear description of the bug.

## Steps to reproduce

1.
2.
3.

## Expected behavior

## Actual behavior

## Environment

- OS:
- Terminal:
- `lazygh` version:
- `gh` version:
```

- [ ] **Step 2: Create `.github/ISSUE_TEMPLATE/feature_request.md`**

```markdown
---
name: Feature request
about: Suggest an idea for lazygh
title: "[Feature]: "
labels: [enhancement]
---

## Problem / motivation

What problem would this solve?

## Proposed solution

## Alternatives considered

## Additional context
```

- [ ] **Step 3: Create `.github/ISSUE_TEMPLATE/config.yml`**

```yaml
blank_issues_enabled: false
```

- [ ] **Step 4: Create `.github/PULL_REQUEST_TEMPLATE.md`**

```markdown
## Summary

<!-- What does this PR do and why? -->

## Related issue

<!-- e.g. Closes #123 -->

## Changes

-

## Checklist

- [ ] Tests pass locally (`make test`)
- [ ] Lint is clean (`make lint`)
- [ ] Docs updated if needed
```

- [ ] **Step 5: Create `.github/CODEOWNERS`**

```text
# Default owner for everything in this repo
* @ChristopherBilg
```

- [ ] **Step 6: Verify**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/ISSUE_TEMPLATE/config.yml')); print('ok')"` (if PyYAML present; else GitHub validates on push).
Run: `grep -q '^\* @ChristopherBilg$' .github/CODEOWNERS && echo OK`
Expected: `OK`.

- [ ] **Step 7: Commit**

```bash
git add .github/ISSUE_TEMPLATE/ .github/PULL_REQUEST_TEMPLATE.md .github/CODEOWNERS
git commit -m "chore: add issue/PR templates and CODEOWNERS

Add bug-report and feature-request issue templates (blank issues
disabled), a pull-request template with a checklist, and a CODEOWNERS
assigning review to @ChristopherBilg.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## After all tasks

- Push the branch; PR #9 updates. Confirm the new CI workflow runs and both jobs pass on the PR.
- **Manual follow-ups (cannot be done via git):** enable **Private Vulnerability Reporting** (repo Settings → Code security) so `SECURITY.md` works; optionally add the `test` and `lint` checks to branch-protection required checks on `main`.

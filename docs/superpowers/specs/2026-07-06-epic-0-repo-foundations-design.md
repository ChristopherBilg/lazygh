# Design: Epic 0 — Repository Foundations & Project Hygiene (README roadmap entry)

- **Issue:** N/A — roadmap authoring
- **Epic:** 0 — Repository Foundations & Project Hygiene (new)
- **Date:** 2026-07-06
- **Status:** Approved
- **Branch:** `docs/epic-0-repo-foundations` (to be created)

## Context

`README.md` is the project's architectural roadmap: a prose intro followed by
`## Epic 1` … `## Epic 5`, each with a title (using `&`), a one-line description, and
bold sub-item groups with nested bullets. It currently jumps straight into feature and
architecture work (Epic 1) without ever describing the foundational repository setup that
should precede all of it.

The repository reflects that gap. Today it contains only:

- `.github/dependabot.yml` (the sole `.github` file — no workflows, no templates),
- `README.md`, `go.mod`, `go.sum`, `cmd/`, `internal/`, `docs/`.

There is **no** `LICENSE`, **no** `.gitignore`, no CI workflow, and none of the standard
community-health or contributor files a public repository is expected to have.

The user wants a new **Epic 0** at the top of the roadmap capturing this foundational
work: CI, standard GitHub files, and general repo hygiene.

## Goals

- Add a `## Epic 0: Repository Foundations & Project Hygiene` section to `README.md`,
  inserted **before** Epic 1, matching the existing epic format exactly.
- Cover four sub-item groups: a CI quality gate, community-health/governance files,
  GitHub templates & review routing, and developer tooling & repo hygiene.
- Reword Epic 5 so its CI responsibility does not duplicate Epic 0's quality gate.

## Non-goals / Out of scope

- **Actually creating** the files, workflows, or configs the epic describes. This change
  edits the roadmap only; each bullet is downstream implementation work.
- **Onboarding extras** — `SUPPORT.md`, `FUNDING.yml`, and a `CHANGELOG` policy were
  considered and deliberately excluded to keep Epic 0 focused.
- Editing any epic other than the Epic 5 de-duplication.

## Key decisions

1. **CI ownership split (user-confirmed).** Epic 0 owns the *day-one quality gate* —
   lint + test + build on every pull request / push to `main`. Epic 5 keeps
   *release/distribution* (`goreleaser`, cross-compilation, Homebrew tap) and its deeper
   testing suite. This is why Epic 5's "run `go test` + `golangci-lint` on every push"
   bullet is reworded rather than left in place: that gate now lives in Epic 0.
2. **Standard-file scope (user-confirmed).** Of the four proposed *standard-file*
   categories, include three — community-health files, GitHub templates & routing, and
   developer tooling & hygiene — and drop "onboarding extras" (see Non-goals). Together
   with the CI quality gate from decision 1, these are Epic 0's four sub-item groups.
3. **License = MIT (user-confirmed).** Recorded as the suggested license in the epic text;
   MIT is the common permissive default for a CLI tool.
4. **Format & placement.** Match the existing epic style (title with `&`, one-line intro,
   bold sub-item groups with nested bullets) and insert as `Epic 0` before Epic 1, so the
   numbering signals "do this first."

## Proposed Epic 0 content (verbatim insertion)

```markdown
## Epic 0: Repository Foundations & Project Hygiene
Before writing feature code, establish the baseline scaffolding, quality gates, and contributor-facing files that every production repository needs.

* **Continuous Integration Quality Gate:**
    * Add a GitHub Actions workflow (`.github/workflows/ci.yml`) triggered on every pull request and push to `main`.
    * Run `go build ./...`, `go test ./...`, `go vet ./...`, and `golangci-lint run` as required status checks.
    * Cache Go modules to keep runs fast; pin action versions (SHA) for reproducibility.
    * Enforce `gofmt`/`goimports` formatting so style is verified automatically.
* **Community Health & Governance Files:**
    * Add an OSI-approved `LICENSE` (MIT) so usage terms are unambiguous.
    * Write `CONTRIBUTING.md` covering local setup, branch/PR conventions, and how to run tests + lint locally.
    * Add `CODE_OF_CONDUCT.md` (Contributor Covenant) to set community expectations.
    * Add `SECURITY.md` describing supported versions and the private vulnerability-disclosure process.
* **GitHub Templates & Review Routing:**
    * Add issue templates under `.github/ISSUE_TEMPLATE/` for bug reports and feature requests, plus a `config.yml` to route questions elsewhere.
    * Add `.github/PULL_REQUEST_TEMPLATE.md` with a checklist (tests pass, lint clean, docs updated).
    * Add `.github/CODEOWNERS` so reviews are auto-requested from the right owners.
* **Developer Tooling & Repo Hygiene:**
    * Add a Go `.gitignore` (compiled binaries, build output, editor/OS cruft, local `.local/state/lazygh` logs).
    * Add `.editorconfig` to standardize indentation and line endings across editors.
    * Add `.golangci.yml` pinning the linter set and rules that CI enforces.
    * Add a `Makefile` exposing common targets: `build`, `test`, `lint`, `run`, `fmt`.
    * (Dependabot is already configured via `.github/dependabot.yml`.)
```

## Epic 5 edit (de-duplication)

Under Epic 5's **Automated Release Pipeline**, replace the first bullet.

Before:

```markdown
    * Set up GitHub Actions to run `go test` and `golangci-lint` on every push.
```

After:

```markdown
    * Reuse the CI quality gate from Epic 0 as a required check, then trigger the release workflow on version tags (`v*`).
```

The `goreleaser` bullet and the entire **Testing Suite** sub-item are unchanged.

## Verification

This is a documentation change, so verification is a read-through, not a test run:

- `README.md` renders as valid Markdown; Epic 0 appears before Epic 1 and reads in the
  same voice/format as the other epics.
- No responsibility is duplicated between Epic 0 and Epic 5 (the CI gate appears once, in
  Epic 0; release/distribution appears once, in Epic 5).
- Every path referenced in Epic 0 (`.github/workflows/ci.yml`, `.github/ISSUE_TEMPLATE/`,
  `.github/CODEOWNERS`, etc.) is spelled consistently and matches GitHub conventions.

## File-by-file change summary

- `README.md` → insert `## Epic 0` section before `## Epic 1`; reword one bullet under
  Epic 5's Automated Release Pipeline. No other epics touched.

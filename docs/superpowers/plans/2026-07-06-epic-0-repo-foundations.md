# Epic 0 Roadmap Entry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new "Epic 0: Repository Foundations & Project Hygiene" section to `README.md` (before Epic 1) and reword Epic 5's overlapping CI bullet so responsibilities don't duplicate.

**Architecture:** Documentation-only edit to a single file, `README.md`. Insert one new `## Epic 0` section that matches the existing epic format (title with `&`, one-line intro, bold sub-item groups with nested bullets), then replace one bullet under Epic 5's "Automated Release Pipeline". No code, config, or other files change — the bullets *describe* downstream work; they do not perform it.

**Tech Stack:** Markdown.

**Spec:** `docs/superpowers/specs/2026-07-06-epic-0-repo-foundations-design.md`

> **Note on TDD:** This is a prose/roadmap change with no runtime behavior, so there is no unit test to write first. The red-green cycle is replaced by **structural verification** (grep-based checks on `README.md`) run after the edit and before commit.

---

### Task 1: Add Epic 0 and de-duplicate Epic 5 in README

**Goal:** `README.md` gains an Epic 0 section before Epic 1, and Epic 5's first "Automated Release Pipeline" bullet no longer duplicates the CI quality gate.

**Files:**
- Modify: `README.md` (insert before the `## Epic 1:` heading, ~line 5; edit the first bullet under Epic 5's "Automated Release Pipeline", ~line 69)

**Acceptance Criteria:**
- [ ] A `## Epic 0: Repository Foundations & Project Hygiene` section exists and appears **before** `## Epic 1`.
- [ ] Epic 0 contains exactly four bold sub-item groups: Continuous Integration Quality Gate, Community Health & Governance Files, GitHub Templates & Review Routing, Developer Tooling & Repo Hygiene.
- [ ] The `LICENSE` bullet names MIT.
- [ ] Epic 5's old bullet ("Set up GitHub Actions to run `go test` and `golangci-lint` on every push.") is gone, replaced by the Epic-0-reuse bullet.
- [ ] Epics 1–5 and their content are otherwise unchanged; the intro paragraph is unchanged.

**Verify:**
```bash
# 1. Epic 0 exists and is first in the epic order:
grep -n '^## Epic' README.md
#   → Epic 0 line number < Epic 1 line number; order 0,1,2,3,4,5

# 2. All four Epic 0 sub-groups present:
grep -c -e 'Continuous Integration Quality Gate' -e 'Community Health & Governance Files' \
        -e 'GitHub Templates & Review Routing' -e 'Developer Tooling & Repo Hygiene' README.md
#   → 4

# 3. Old Epic 5 CI bullet removed, new bullet present:
grep -c 'on every push' README.md            # → 0
grep -c 'Reuse the CI quality gate from Epic 0' README.md   # → 1

# 4. Markdown still parses / renders (optional if a linter is installed):
#    npx --yes markdownlint-cli README.md   (skip if not available; do a visual read instead)
```

**Steps:**

- [ ] **Step 1: Insert the Epic 0 section immediately before the `## Epic 1:` heading**

Use an exact-match edit. `old_string` (the current Epic 1 heading line):

```
## Epic 1: Architectural Foundations & State Management
```

`new_string` (the full Epic 0 block, a blank line, then the original Epic 1 heading):

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
    * Extend Dependabot (already covering Go modules via `.github/dependabot.yml`) to also update GitHub Actions and group minor/patch bumps.

## Epic 1: Architectural Foundations & State Management
```

- [ ] **Step 2: Reword the first bullet under Epic 5's "Automated Release Pipeline"**

Use an exact-match edit. `old_string`:

```
    * Set up GitHub Actions to run `go test` and `golangci-lint` on every push.
```

`new_string`:

```
    * Reuse the CI quality gate from Epic 0 as a required check, then trigger the release workflow on version tags (`v*`).
```

- [ ] **Step 3: Verify structure**

Run the four checks from the **Verify** block above. Expected:
- Check 1: `grep -n '^## Epic'` lists Epic 0 first, then 1,2,3,4,5 in ascending order.
- Check 2: prints `4`.
- Check 3: `on every push` → `0`; `Reuse the CI quality gate from Epic 0` → `1`.
- Then do a quick visual read of the Epic 0 block to confirm it reads in the same voice as the other epics.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add Epic 0 (repository foundations) and de-dup Epic 5 CI bullet

Adds a foundational 'Epic 0: Repository Foundations & Project Hygiene'
section to the roadmap covering the CI quality gate, community-health
files, GitHub templates/routing, and developer tooling. Rewords Epic 5's
first Automated Release Pipeline bullet to reuse Epic 0's gate instead of
re-declaring it.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

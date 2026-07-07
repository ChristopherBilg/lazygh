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

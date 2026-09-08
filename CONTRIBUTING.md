# Contributing to Vane

Thanks for considering a contribution. Vane is pre-1.0.0 and still shaping
its public API, so bug reports and real-world usage feedback are as valuable
as code.

## Before you start

- **Bugs** → [GitHub Issues](https://github.com/FilipeJohansson/vane/issues).
- **Questions, ideas, "how would I..."** → [GitHub Discussions](https://github.com/FilipeJohansson/vane/discussions).
- **Security vulnerabilities** → do not open a public issue. See
  [SECURITY.md](SECURITY.md).
- For a change larger than a small fix, open an issue first to agree on the
  approach before writing code. It avoids a PR being rejected after the
  work is already done.

## Project setup

Requires Go 1.24+, Node.js 20+, and [pnpm](https://pnpm.io/).

```bash
git clone https://github.com/FilipeJohansson/vane.git
cd vane
go build -o vane .
corepack enable && pnpm install
```

The repo is a [Go workspace](go.work): the root module (`core`, `internal`,
the CLI in `main.go`) plus every app under `examples/` and `vane-page/` (the
docs site) as separate modules that depend on the root one.

## Running checks

Run these before opening a PR — they mirror what CI runs in
[.github/workflows/ci.yml](.github/workflows/ci.yml):

```bash
go build ./...
go vet ./...
go run ./internal/apisurface  # public API surface check, see below
gofmt -l .              # must print nothing
go mod verify
make lint               # golangci-lint
make security           # gosec
go test -race ./...     # unit tests
make test-dom           # core/ and core/router/ against jsdom (js/wasm build tag)
make test-e2e           # Playwright, real browsers — needs `pnpm install` above
```

`make test-e2e` builds the CLI, compiles the `.vane` test app under
`tests/e2e/app` to WASM, and runs it against Chromium, Firefox, and WebKit.
It's slower than the rest; run it at least once before a PR that touches
`core/`, `internal/compiler/`, or the router.

### Public API surface check

`core`, `core/router`, `core/signal`, and `core/domattrs` are Vane's frozen
public API (see [API_STABILITY.md](API_STABILITY.md)). Every exported
symbol in those packages, including exported methods and struct fields,
is tracked in a golden file per package under [goldens/](goldens/). CI
regenerates each golden from current source and fails the PR if it differs,
whether the change is a rename, a removal, a signature change, or a plain
addition.

If your PR intentionally changes one of those packages' public surface,
regenerate the goldens and commit the result:

```bash
go run ./internal/apisurface -write
git add goldens/
```

Without `-write`, the same command only checks the surface against the
committed goldens and exits non-zero if they don't match.

Optionally, `make install-hooks` sets `git config core.hooksPath .githooks`
so `git push` runs the CI workflow locally first via
[act](https://github.com/nektos/act) (falls back to a no-op if `act` isn't
installed).

## Coding conventions

- Format with `gofmt`; CI fails on unformatted code.
- Linting is `golangci-lint` per [.golangci.yml](.golangci.yml) — fix
  warnings rather than adding exclusions, unless the existing exclusion
  rules already justify the case (see the comments in that file).
- Prefer small, focused PRs. A bug fix shouldn't carry an unrelated
  refactor.
- New behavior in `core/` or `core/router/` needs test coverage — a Go
  unit test, a jsdom test under the `js/wasm` build tag, or a Playwright
  spec under `tests/e2e/specs/`, whichever actually exercises the change
  (see [Makefile](Makefile) comments for what each layer covers and its
  known gaps, e.g. jsdom has no layout engine).
- Compiler changes (`internal/compiler/`) that affect error output need a
  test asserting the exact message/position, following the existing
  `TestError*` tests in `internal/compiler/compiler_test.go`.

## Commit and PR style

Commit subjects in this repo follow
`<type>(<scope>): <description>` or plain `<type>: <description>`, matching
Conventional Commits `type`s (`feat`, `fix`, `docs`, `test`, `chore`, ...) —
check `git log` for examples. Branch names follow the same `type/` prefix,
e.g. `fix/router-cleanup`, `docs/security-policy`.

PRs are squash-merged, so **the PR title becomes the commit message on
`master`** — individual commit messages inside the PR are discarded. The
title must follow the same Conventional Commits format; a CI check rejects
titles that don't. [CHANGELOG.md](CHANGELOG.md) is regenerated automatically
after every merge to `master` by
[.github/workflows/changelog.yml](.github/workflows/changelog.yml), using
[git-cliff](https://git-cliff.org/) (config: [cliff.toml](cliff.toml)). The footers below are how a PR reaches a specific
changelog section
instead of the generic type-based one.

When opening a PR:

1. Title it `<type>(<scope>): <description>` or `<type>: <description>`.
2. Is this a breaking change? If yes, mark it in the title with a `!`
   (`fix!: ...`) and add a `BREAKING CHANGE:` footer in the description.
3. Does it deprecate a public API? Add a `Deprecated: <symbol> - <reason>`
   footer. Does it fix a security issue? Add a `Security: <description>`
   footer instead. Either routes the entry into its own changelog section.
4. Describe what changed and why, not just what.
5. Link the issue it closes, if any.
6. Make sure `go build ./...`, `go test -race ./...`, `make test-dom`, and
   `go run ./internal/apisurface` pass locally; CI will run the full matrix
   (including `make test-e2e`) automatically.
7. Expect review feedback — Vane is a small project, response time varies.

## License

By contributing, you agree that your contributions will be licensed under
the project's [MIT License](LICENSE).

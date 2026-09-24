# Tandem

Work on one change across many repos. A **change** (e.g. `ABC-123`) has one
**leg** per repo: a git worktree on a shared branch, its local checks and its
PR. Tandem works out merge order from `go.mod`, `composer.json` and
`package.json`, and keeps a "Related PRs" block in every PR.

## Install

```sh
go install github.com/jonezzyboy/tandem/cmd/td@latest   # or: go build -o ~/bin/td ./cmd/td
```

Needs `git` and an authenticated `gh`. The repo is private, so `go install` needs
`GOPRIVATE=github.com/jonezzyboy/*`.

## Use

```sh
td start ABC-123 proto orchestrator acme/monolith --title "Add request retries"
cd "$(td path ABC-123 orchestrator)"   # commit in the worktrees as usual
td status             # every leg in merge order: local state, PR, CI, review
td check              # local checks per leg, legs in parallel
td sync               # fetch all; rebase clean legs onto their base
td pr --draft --body-file notes.md --reviewer alice,bob
td link orchestrator monolith   # declare an edge manifests can't see
```

Inside a change's directory the ID can be left out; elsewhere it can too when
only one change exists.

Repos are found as `<root>/<vendor>/<repo>` under `$TANDEM_ROOT` (default
`~/code`). Changes and their worktrees live in `$TANDEM_HOME` (default
`~/code/.tandem/<ID>/<repo>`), with state in `change.json`.

### `td pr`

Pushes each leg that has commits (never force-pushing unless you pass
`--force-with-lease`), opens a PR for legs without one, then rewrites the
Related PRs block in all of them. The block sits between
`<!-- tandem:related -->` markers; the rest of the description is left alone.
`--dry-run` prints the plan only.

### Checks

Detected from the worktree root and its immediate subdirectories:

| Found | Runs |
|---|---|
| `go.mod` | `go build -o /dev/null ./...`, `go vet ./...`, `go test ./...` |
| `composer.json` + `phpstan.neon(.dist)` / `phpunit.xml(.dist)` | `vendor/bin/phpstan analyse`, `vendor/bin/phpunit` |
| `package.json` scripts `typecheck`, `lint`, `test` | `<npm\|pnpm\|yarn> run <script>` with `CI=true` |
| `buf.yaml` | `buf lint` |

A fresh worktree has no `vendor/` or `node_modules/`; those checks show as
skipped until you install. A `.tandem.yml` at the repo root replaces
detection:

```yaml
checks:
  - name: phpunit
    dir: backend
    run: vendor/bin/phpunit --testsuite unit
```

## Layout

`internal/core` computes a change's state (graph, per-leg git + GitHub
snapshot) and is shared by the CLI and the planned Wails desktop app.
`cmd/td` → `internal/cli` renders it.

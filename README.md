# Tandem

Work on one change across many repos. A **change** (e.g. `ABC-123`) has one
**leg** per repo: a git worktree on a shared branch, its local checks and its
PR. Tandem works out merge order from `go.mod`, `composer.json` and
`package.json`, and keeps a "Related PRs" block in every PR.

## Install

```sh
brew install --cask Jonezzyboy/tandem/tandem-repos
```

The cask is `tandem-repos`, not `tandem`: homebrew-cask's own `tandem` is an
unrelated app (a virtual office), and any unqualified `brew … tandem` gets that one.

Installs the desktop app and the `td` CLI together (`td` ships inside
`Tandem.app` and the cask links it onto your PATH), so both are always the same
version. Universal (Apple Silicon and Intel). homebrew-core's unrelated `td`
formula (a to-do list) installs the same command; uninstall it first, since brew
will not link over it.

`gh` comes along as a cask dependency, but you still need to be signed in
(`gh auth login`); the app has no login of its own. `brew upgrade --cask tandem-repos`
picks up new releases.

The build is ad-hoc signed rather than notarised with an Apple Developer ID, so
Gatekeeper would block the first launch. The cask clears the quarantine attribute in
`postflight_steps` so the install just works. That trades away Gatekeeper's check on
this app; the real fix is a Developer ID signature and notarisation in the release
workflow, after which the `postflight_steps` block in `packaging/cask.rb.tmpl` goes.

The CLI alone, or on Linux:

```sh
go install github.com/jonezzyboy/tandem/cmd/td@latest   # or a td_* archive from the release
```

Both need `git` and an authenticated `gh`.

## Use

```sh
td start ABC-123 proto orchestrator acme/monolith --title "Add request retries"
cd "$(td path ABC-123 orchestrator)"   # commit in the worktrees as usual
td status             # every leg in merge order: local state, PR, CI, review
td check              # local checks per leg, legs in parallel
td sync               # fetch all; rebase clean legs onto their base
td pr --draft --body-file notes.md --reviewer alice,bob
td link orchestrator monolith   # declare an edge manifests can't see
td pin                # point downstream Go legs at their upstream's pushed commit
td merge              # merge the PRs in dependency order (asks first)
td clean              # remove worktrees and branches of changes that have landed (asks first)
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

### `td merge`

A merge train. It first checks every PR: drafts, missing approvals and failing
checks stop it before anything merges. Failing CI is tolerated on a leg the
train will re-pin, since the pin is often the fix. It shows the plan and asks.
Then, level by level:

1. Wait for the leg's CI to finish green, then merge it (`--method squash`, the
   default, or `merge` / `rebase`), and wait for GitHub to report it merged.
2. For each Go leg that requires a module just merged, `go get` it at the merge
   commit, commit `go.mod`/`go.sum`, and push. That leg only merges once CI has
   run and passed on the pushed commit.

Stopping part-way (Ctrl-C, a red check, a timeout) is safe: merged legs stay
merged, and running `td merge` again skips them and carries on. Composer and npm
edges order the train but are not re-pinned.

### `td pin`

For every Go module one leg provides and another requires, runs
`go get <module>@<upstream HEAD>` in the downstream leg and commits only
`go.mod`/`go.sum`. The upstream must be pushed first (`td pr` pushes), because
`go get` fetches the commit from its origin, so private modules need `GOPRIVATE`
set as usual. `--no-commit` leaves the change for you to commit.

### `td clean`

Lists every change whose PRs have all merged or closed, with each worktree,
local branch and file it would remove, and deletes only after a yes. A change
with uncommitted or unpushed work, or a PR still open, is kept and the reason
shown. A branch whose PR closed unmerged is kept. Worktrees are removed with
plain `git worktree remove`, which refuses a modified one.

`td merge` and `td clean` need a terminal to confirm, or `--yes`.

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

## Desktop app

`desktop/` is a Wails (Go + system WebView) app over the same core: changes
sidebar, Change view (merge order, per-leg git/PR/CI/review, streamed local
checks), a PR composer that previews then publishes, a review Inbox, and a
New change picker. Views are served from a cache (persisted as
`.view.json` beside each change) and refreshed in the background — local git
state every 4s for the change on screen, GitHub every minute.

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
scripts/build-app.sh             # → desktop/build/bin/Tandem.app, with td inside
cd desktop && wails dev          # hot reload; also served at http://localhost:34115
```

`go test ./...` in `desktop/` needs `frontend/dist` to exist (it is embedded), so
run `scripts/build-app.sh` or `npm run build` in `frontend/` first.

Shortcuts: ⌘N new change, ⌘R refresh, ⌘0 inbox, ⌘1–9 jump to a change.
`TANDEM_EDITOR` (default `code`) is what "Open in editor" runs.

## Releasing

A published GitHub release is the single trigger: `.github/workflows/release.yml`
builds the `td` archives and the universal `Tandem.app` zip, attaches them to that
release, and rewrites `Casks/tandem-repos.rb` in
[Jonezzyboy/homebrew-tandem](https://github.com/Jonezzyboy/homebrew-tandem) with the
new version and checksum. What is tagged on GitHub is what `brew` serves.

```sh
scripts/set-version.sh 0.3.0
git commit -am "Release 0.3.0" && git push
gh release create v0.3.0 --generate-notes
```

The tag has to read `v<version>`. The version lives in `desktop/wails.json` and
`desktop/frontend/package.json`; the workflow refuses to build when either disagrees
with the tag. It needs a `TAP_TOKEN` repo secret, a PAT with write access to the tap
repo, since the default workflow token cannot push across repositories.

## Layout

`internal/core` computes a change's state and runs its operations (start,
sync, checks, PR plan/publish); `cmd/td` → `internal/cli` and `desktop/` both
render it. `desktop/` is its own Go module so the CLI never links Wails.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

// goupGH serves <repo>.json from FAKE_GH_DIR with __HEAD__ replaced by the
// branch's pushed commit.
const goupGH = `#!/bin/sh
f="$FAKE_GH_DIR/$(basename "$PWD").json"
[ "$1 $2" = "pr view" ] || { echo "unexpected gh $*" >&2; exit 1; }
[ -f "$f" ] || { echo "no pull requests found for branch" >&2; exit 1; }
sed "s/__HEAD__/$(git rev-parse @{u})/" "$f"
`

// goupGo stands in for go get mod@rev: it requires mod at a pseudo-version of
// rev and logs the call to FAKE_GO_LOG.
const goupGo = `#!/bin/sh
[ "$1" = get ] || { echo "unexpected go $*" >&2; exit 1; }
echo "$(basename "$PWD") $2" >> "$FAKE_GO_LOG"
mod=${2%@*}
rev=$(echo "${2#*@}" | cut -c1-12)
sed "s#^require $mod .*#require $mod v0.0.0-20260101000000-$rev#" go.mod > go.mod.tmp && mv go.mod.tmp go.mod
`

type goupFixture struct {
	t     *testing.T
	root  string
	ghDir string
	goLog string
	app   *App
	c     *change.Change

	mu     sync.Mutex
	events []ActivityUpdate
}

func sh(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func put(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

// newGoupFixture starts DEV-1 over pkg-hosted and two repos requiring it,
// each with a pushed commit and an open PR.
func newGoupFixture(t *testing.T) *goupFixture {
	base := t.TempDir()
	f := &goupFixture{t: t, root: filepath.Join(base, "code"), ghDir: filepath.Join(base, "gh"), goLog: filepath.Join(base, "go.log")}
	bin := filepath.Join(base, "bin")
	put(t, filepath.Join(bin, "gh"), goupGH, 0o755)
	put(t, filepath.Join(bin, "go"), goupGo, 0o755)
	put(t, f.goLog, "", 0o644)
	if err := os.MkdirAll(f.ghDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_DIR", f.ghDir)
	t.Setenv("FAKE_GO_LOG", f.goLog)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	requires := "\n\nrequire example.com/pkg-hosted v1.0.0\n"
	repos := []workspace.Repo{
		f.repo("pkg-hosted", "module example.com/pkg-hosted\n\ngo 1.26\n"),
		f.repo("app-hosted", "module example.com/app-hosted\n\ngo 1.26"+requires),
		f.repo("hosted-account", "module example.com/hosted-account\n\ngo 1.26"+requires),
	}
	store := change.Store{Home: filepath.Join(f.root, ".tandem")}
	c, results, err := core.Start(context.Background(), store, "DEV-1", "Downloads", repos, core.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatal(r.Err)
		}
	}
	for _, l := range c.Legs {
		put(t, filepath.Join(l.Dir(), "work.txt"), "work", 0o644)
		sh(t, l.Dir(), "git", "add", ".")
		sh(t, l.Dir(), "git", "commit", "--quiet", "-m", "work")
		sh(t, l.Dir(), "git", "push", "--quiet", "-u", "origin", "DEV-1")
	}
	f.c = c
	f.pr("pkg-hosted", 46, "OPEN", "")
	f.pr("app-hosted", 87, "OPEN", "")
	f.pr("hosted-account", 39, "OPEN", "")

	f.app = NewApp(store, nil)
	t.Cleanup(f.waitIdle)
	f.app.ctx = context.Background()
	f.app.emit = func(event string, data any) {
		if u, ok := data.(ActivityUpdate); ok && event == "activity" {
			f.mu.Lock()
			f.events = append(f.events, u)
			f.mu.Unlock()
		}
	}
	return f
}

// waitIdle lets background refreshes (CommitPins starts one) finish before
// the temp dirs they write into are removed.
func (f *goupFixture) waitIdle() {
	idle := 0
	for i := 0; i < 500 && idle < 20; i++ {
		f.app.mu.Lock()
		busy := len(f.app.inflight) > 0
		f.app.mu.Unlock()
		if busy {
			idle = 0
		} else {
			idle++
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (f *goupFixture) repo(name, gomod string) workspace.Repo {
	dir := filepath.Join(f.root, "chargehive", name)
	bare := filepath.Join(f.root, "..", "origins", name+".git")
	sh(f.t, filepath.Dir(f.root), "git", "init", "--quiet", "--bare", "-b", "main", bare)
	sh(f.t, filepath.Dir(f.root), "git", "init", "--quiet", "-b", "main", dir)
	put(f.t, filepath.Join(dir, "go.mod"), gomod, 0o644)
	sh(f.t, dir, "git", "add", ".")
	sh(f.t, dir, "git", "commit", "--quiet", "-m", "init")
	sh(f.t, dir, "git", "remote", "add", "origin", bare)
	sh(f.t, dir, "git", "push", "--quiet", "-u", "origin", "main")
	sh(f.t, dir, "git", "remote", "set-head", "origin", "main")
	return workspace.Repo{Name: "chargehive/" + name, Path: dir}
}

func (f *goupFixture) pr(repo string, n int, state, mergeSHA string) {
	merge := "null"
	if mergeSHA != "" {
		merge = fmt.Sprintf(`{"oid":"%s"}`, mergeSHA)
	}
	put(f.t, filepath.Join(f.ghDir, repo+".json"), fmt.Sprintf(
		`{"number":%d,"url":"https://github.com/chargehive/%s/pull/%d","state":"%s","body":"","isDraft":false,"reviewDecision":"REVIEW_REQUIRED","headRefOid":"__HEAD__","statusCheckRollup":[],"mergeCommit":%s}`,
		n, repo, n, state, merge), 0o644)
}

// merge lands pkg-hosted's branch on its origin's main as a squash would,
// and reports the PR merged at that commit.
func (f *goupFixture) merge() string {
	pkg := f.leg("pkg-hosted")
	sh(f.t, pkg.Dir(), "git", "push", "--quiet", "origin", "DEV-1:main")
	sha := sh(f.t, pkg.Dir(), "git", "rev-parse", "DEV-1")
	f.pr("pkg-hosted", 46, "MERGED", sha)
	return sha
}

func (f *goupFixture) leg(name string) *change.Leg {
	l, err := f.c.Leg("chargehive/" + name)
	if err != nil {
		f.t.Fatal(err)
	}
	return l
}

func (f *goupFixture) refresh(remote bool) core.ChangeView {
	v, err := f.app.buildView("DEV-1", remote)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *goupFixture) goCalls() []string {
	data, _ := os.ReadFile(f.goLog)
	return strings.Fields(strings.ReplaceAll(string(data), "\n", " "))
}

func pinsOf(v core.ChangeView, name string) []core.PinView {
	for _, l := range v.Legs {
		if l.Name == name {
			return l.Pins
		}
	}
	return nil
}

func pinnedGoMod(t *testing.T, l *change.Leg, sha string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(l.Dir(), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(data), "require example.com/pkg-hosted v0.0.0-20260101000000-"+sha[:12])
}

func TestUpstreamMergeTriggersGoupThenCommitAndPush(t *testing.T) {
	f := newGoupFixture(t)
	downstream := []string{"app-hosted", "hosted-account"}

	v := f.refresh(true)
	for _, name := range downstream {
		if p := pinsOf(v, name); len(p) != 0 {
			t.Fatalf("%s has pins before anything merged: %+v", name, p)
		}
	}
	if calls := f.goCalls(); len(calls) != 0 {
		t.Fatalf("go get ran before anything merged: %v", calls)
	}

	sha := f.merge()
	v = f.refresh(true)
	for _, name := range downstream {
		l := f.leg(name)
		if !pinnedGoMod(t, l, sha) {
			t.Errorf("%s: go.mod not updated to the merge commit", name)
		}
		if out := sh(t, l.Dir(), "git", "status", "--porcelain"); out != "M go.mod" {
			t.Errorf("%s: want only go.mod modified and uncommitted, got %q", name, out)
		}
		p := pinsOf(v, name)
		if len(p) != 1 || !p[0].Applied || p[0].Rev != sha || p[0].Upstream != "pkg-hosted" {
			t.Errorf("%s: view pins = %+v", name, p)
		}
	}
	if p := pinsOf(v, "pkg-hosted"); len(p) != 0 {
		t.Errorf("pkg-hosted has pins: %+v", p)
	}
	f.mu.Lock()
	if len(f.events) != 2 || !strings.Contains(f.events[0].Text, "pkg-hosted merged") || f.events[0].Tone != "ok" {
		t.Errorf("activity = %+v", f.events)
	}
	f.mu.Unlock()

	f.refresh(true)
	f.refresh(false)
	if calls := f.goCalls(); len(calls) != 4 {
		t.Errorf("go get should run once per downstream, got %v", calls)
	}

	g, _ := core.BuildGraph(f.c)
	var blocked int
	for _, tc := range core.TrainPreflight(context.Background(), f.c, g) {
		for _, p := range tc.Problems {
			if strings.Contains(p, "not committed") {
				blocked++
			}
		}
	}
	if blocked != 2 {
		t.Errorf("train should refuse both uncommitted pins, got %d", blocked)
	}

	results, err := f.app.CommitPins("DEV-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("commit results = %+v", results)
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("%s: %s", r.Leg, r.Message)
		}
	}
	for _, name := range downstream {
		l := f.leg(name)
		if msg := sh(t, l.Dir(), "git", "log", "-1", "--format=%s"); msg != "Pin example.com/pkg-hosted to merged "+sha[:12] {
			t.Errorf("%s: commit message = %q", name, msg)
		}
		if files := sh(t, l.Dir(), "git", "show", "--name-only", "--format=", "HEAD"); files != "go.mod" {
			t.Errorf("%s: pin commit touched %q", name, files)
		}
		if sh(t, l.Dir(), "git", "rev-parse", "HEAD") != sh(t, l.Dir(), "git", "rev-parse", "@{u}") {
			t.Errorf("%s: pin not pushed", name)
		}
		if out := sh(t, l.Dir(), "git", "status", "--porcelain"); out != "" {
			t.Errorf("%s: left %q", name, out)
		}
	}

	v = f.refresh(true)
	for _, name := range downstream {
		if p := pinsOf(v, name); len(p) != 0 {
			t.Errorf("%s: pins remain after commit: %+v", name, p)
		}
	}
	for _, tc := range core.TrainPreflight(context.Background(), f.c, g) {
		for _, p := range tc.Problems {
			if strings.Contains(p, "not committed") {
				t.Errorf("%s: %s", tc.Leg.Name(), p)
			}
		}
	}
	if calls := f.goCalls(); len(calls) != 4 {
		t.Errorf("go get ran again after commit: %v", calls)
	}
}

func TestGoupLeavesUnreadyReposForCommit(t *testing.T) {
	f := newGoupFixture(t)
	app, account := f.leg("app-hosted"), f.leg("hosted-account")
	sh(t, account.Dir(), "git", "switch", "--quiet", "main")
	put(t, filepath.Join(app.Dir(), "go.mod"), "module example.com/app-hosted\n\ngo 1.26\n\nrequire example.com/pkg-hosted v1.0.0\n// mine\n", 0o644)

	sha := f.merge()
	v := f.refresh(true)
	if calls := f.goCalls(); len(calls) != 0 {
		t.Errorf("go get ran in a repo with its own go.mod edits or off its branch: %v", calls)
	}
	for _, name := range []string{"app-hosted", "hosted-account"} {
		if p := pinsOf(v, name); len(p) != 1 || p[0].Applied || p[0].Rev != sha {
			t.Errorf("%s: pins = %+v, want one pending", name, p)
		}
	}

	results, err := f.app.CommitPins("DEV-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.OK {
			t.Errorf("%s: committed despite %s", r.Leg, map[string]string{"app-hosted": "its own go.mod edits", "hosted-account": "being off its branch"}[r.Leg])
		}
	}

	sh(t, app.Dir(), "git", "checkout", "--", "go.mod")
	sh(t, account.Dir(), "git", "switch", "--quiet", "DEV-1")
	results, err = f.app.CommitPins("DEV-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("%s: %s", r.Leg, r.Message)
		}
	}
	for _, l := range []*change.Leg{app, account} {
		if !pinnedGoMod(t, l, sha) || sh(t, l.Dir(), "git", "rev-parse", "HEAD") != sh(t, l.Dir(), "git", "rev-parse", "@{u}") {
			t.Errorf("%s: not pinned and pushed", l.Name())
		}
	}
}

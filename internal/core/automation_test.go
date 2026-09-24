package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

// fakeGH serves <repo>.json from FAKE_GH_DIR with __HEAD__ replaced by the
// branch's pushed commit, and "merges" by rewriting that file.
const fakeGH = `#!/bin/sh
repo=$(basename "$PWD")
f="$FAKE_GH_DIR/$repo.json"
case "$1 $2" in
"pr view")
  [ -f "$f" ] || { echo "no pull requests found for branch" >&2; exit 1; }
  sed "s/__HEAD__/$(git rev-parse @{u})/" "$f" ;;
"pr merge")
  echo "$repo" >> "$FAKE_GH_DIR/merged"
  sha=$(git rev-parse @{u})
  sed -e 's/"state":"OPEN"/"state":"MERGED"/' -e "s/\"mergeCommit\":null/\"mergeCommit\":{\"oid\":\"$sha\"}/" "$f" > "$f.tmp" && mv "$f.tmp" "$f" ;;
*) echo "unexpected gh $*" >&2; exit 1 ;;
esac
`

// fakeGo records go get by appending a comment to go.mod, standing in for the
// module fetch a real pin needs.
const fakeGo = `#!/bin/sh
[ "$1" = get ] || { echo "unexpected go $*" >&2; exit 1; }
echo "// pinned $2" >> go.mod
`

type fixture struct {
	t     *testing.T
	root  string
	ghDir string
	store change.Store
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) *fixture {
	base := t.TempDir()
	f := &fixture{t: t, root: filepath.Join(base, "code"), ghDir: filepath.Join(base, "gh")}
	f.store = change.Store{Home: filepath.Join(f.root, ".tandem")}
	bin := filepath.Join(base, "bin")
	write(t, filepath.Join(bin, "gh"), fakeGH, 0o755)
	write(t, filepath.Join(bin, "go"), fakeGo, 0o755)
	if err := os.MkdirAll(f.ghDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_DIR", f.ghDir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
	return f
}

func (f *fixture) repo(name, gomod string) workspace.Repo {
	dir := filepath.Join(f.root, "acme", name)
	bare := filepath.Join(f.root, "..", "origins", name+".git")
	run(f.t, filepath.Dir(f.root), "git", "init", "--quiet", "--bare", "-b", "main", bare)
	run(f.t, filepath.Dir(f.root), "git", "init", "--quiet", "-b", "main", dir)
	write(f.t, filepath.Join(dir, "go.mod"), gomod, 0o644)
	run(f.t, dir, "git", "add", ".")
	run(f.t, dir, "git", "commit", "--quiet", "-m", "init")
	run(f.t, dir, "git", "remote", "add", "origin", bare)
	run(f.t, dir, "git", "push", "--quiet", "-u", "origin", "main")
	run(f.t, dir, "git", "remote", "set-head", "origin", "main")
	return workspace.Repo{Name: "acme/" + name, Path: dir}
}

// change starts DEV-1 over proto and api (api requires proto) with one pushed
// commit on each leg.
func (f *fixture) change() *change.Change {
	proto := f.repo("proto", "module example.com/proto\n\ngo 1.26\n")
	api := f.repo("api", "module example.com/api\n\ngo 1.26\n\nrequire example.com/proto v1.0.0\n")
	c, results, err := Start(context.Background(), f.store, "DEV-1", "Retries", []workspace.Repo{proto, api}, StartOptions{})
	if err != nil {
		f.t.Fatal(err)
	}
	for _, r := range results {
		if r.Err != nil {
			f.t.Fatal(r.Err)
		}
	}
	for _, l := range c.Legs {
		f.commit(l, "work.txt", "work")
		run(f.t, l.Dir(), "git", "push", "--quiet", "-u", "origin", "DEV-1")
	}
	return c
}

func (f *fixture) commit(l change.Leg, file, data string) {
	write(f.t, filepath.Join(l.Dir(), file), data, 0o644)
	run(f.t, l.Dir(), "git", "add", ".")
	run(f.t, l.Dir(), "git", "commit", "--quiet", "-m", "change "+file)
}

func (f *fixture) pr(repo string, number int, state, review string, checks string) {
	json := fmt.Sprintf(`{"number":%d,"url":"https://github.com/acme/%s/pull/%d","state":"%s","body":"","isDraft":false,"reviewDecision":"%s","headRefOid":"__HEAD__","statusCheckRollup":[%s],"mergeCommit":null}`,
		number, repo, number, state, review, checks)
	write(f.t, filepath.Join(f.ghDir, repo+".json"), json, 0o644)
}

const (
	green = `{"name":"test","status":"COMPLETED","conclusion":"SUCCESS"}`
	red   = `{"name":"test","status":"COMPLETED","conclusion":"FAILURE"}`
)

func fastTrain(events *[]TrainEvent) TrainOptions {
	return TrainOptions{
		Poll: 5 * time.Millisecond, Settle: 20 * time.Millisecond,
		CheckTimeout: 2 * time.Second, MergeTimeout: 2 * time.Second,
		OnEvent: func(e TrainEvent) { *events = append(*events, e) },
	}
}

func TestTrainMergesInOrderAndPinsDownstream(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	f.pr("proto", 1, "OPEN", "APPROVED", green)
	f.pr("api", 2, "OPEN", "", green)
	g, err := BuildGraph(c)
	if err != nil {
		t.Fatal(err)
	}

	var events []TrainEvent
	if err := RunTrain(context.Background(), c, g, fastTrain(&events)); err != nil {
		t.Fatalf("train: %v\nevents: %+v", err, events)
	}
	merged, _ := os.ReadFile(filepath.Join(f.ghDir, "merged"))
	if string(merged) != "proto\napi\n" {
		t.Errorf("merge order = %q", merged)
	}

	api, _ := c.Leg("api")
	protoSHA := run(t, filepath.Join(f.root, "..", "origins", "proto.git"), "git", "rev-parse", "DEV-1")
	gomod, _ := os.ReadFile(filepath.Join(api.Dir(), "go.mod"))
	if !strings.Contains(string(gomod), "// pinned example.com/proto@"+protoSHA) {
		t.Errorf("api go.mod not pinned to proto's merge commit %s:\n%s", protoSHA, gomod)
	}
	if msg := run(t, api.Dir(), "git", "log", "-1", "--format=%s"); !strings.HasPrefix(msg, "Pin example.com/proto to merged ") {
		t.Errorf("pin commit message = %q", msg)
	}
	if run(t, api.Dir(), "git", "rev-parse", "HEAD") != run(t, api.Dir(), "git", "rev-parse", "@{u}") {
		t.Error("pin commit was not pushed")
	}
	if last := events[len(events)-1]; last.Phase != "done" {
		t.Errorf("last event = %+v", last)
	}

	// A second run finds everything merged and changes nothing.
	events = nil
	if err := RunTrain(context.Background(), c, g, fastTrain(&events)); err != nil {
		t.Fatal(err)
	}
	merged, _ = os.ReadFile(filepath.Join(f.ghDir, "merged"))
	if string(merged) != "proto\napi\n" {
		t.Errorf("rerun merged again: %q", merged)
	}
}

func TestTrainStopsWhenPinnedLegFailsCI(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	f.pr("proto", 1, "OPEN", "APPROVED", green)
	f.pr("api", 2, "OPEN", "APPROVED", red)
	g, _ := BuildGraph(c)

	checks := TrainPreflight(context.Background(), c, g)
	for _, tc := range checks {
		if len(tc.Problems) > 0 {
			t.Fatalf("api's failing CI should be tolerated before its pin: %+v", tc)
		}
	}
	var events []TrainEvent
	err := RunTrain(context.Background(), c, g, fastTrain(&events))
	if err == nil || !strings.Contains(err.Error(), "api: checks failed on #2") {
		t.Fatalf("err = %v", err)
	}
	merged, _ := os.ReadFile(filepath.Join(f.ghDir, "merged"))
	if string(merged) != "proto\n" {
		t.Errorf("merged = %q, want only proto", merged)
	}
}

func TestTrainPreflightBlocks(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	f.pr("proto", 1, "OPEN", "CHANGES_REQUESTED", red)
	g, _ := BuildGraph(c)
	var got []string
	for _, tc := range TrainPreflight(context.Background(), c, g) {
		got = append(got, tc.Leg.Name()+": "+strings.Join(tc.Problems, ", "))
	}
	want := "proto: changes requested, failing checks: test|api: no PR"
	if strings.Join(got, "|") != want {
		t.Errorf("preflight = %q, want %q", strings.Join(got, "|"), want)
	}
	var events []TrainEvent
	if err := RunTrain(context.Background(), c, g, fastTrain(&events)); err == nil || !strings.Contains(err.Error(), "train cannot start") {
		t.Errorf("err = %v", err)
	}
}

func TestPin(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	g, _ := BuildGraph(c)
	proto, _ := c.Leg("proto")
	api, _ := c.Leg("api")

	f.commit(*proto, "more.txt", "unpushed")
	res := Pin(context.Background(), c, g, true)
	if len(res) != 1 || !strings.Contains(res[0].Skipped, "not pushed") {
		t.Fatalf("unpushed upstream: %+v", res)
	}

	run(t, proto.Dir(), "git", "push", "--quiet")
	head := run(t, proto.Dir(), "git", "rev-parse", "HEAD")
	res = Pin(context.Background(), c, g, true)
	if res[0].Err != nil || !res[0].Committed || res[0].Rev != head {
		t.Fatalf("pin: %+v", res[0])
	}
	if out := run(t, api.Dir(), "git", "status", "--porcelain"); out != "" {
		t.Errorf("pin left changes: %q", out)
	}
}

func TestClean(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	f.pr("proto", 1, "MERGED", "APPROVED", green)
	f.pr("api", 2, "OPEN", "", green)
	proto, _ := c.Leg("proto")
	api, _ := c.Leg("api")

	cands, err := CleanCandidates(context.Background(), f.store)
	if err != nil || len(cands) != 1 || cands[0].Ready || !strings.Contains(cands[0].Reason, "api: PR #2 still open") {
		t.Fatalf("open PR should block: %+v %v", cands, err)
	}
	if err := Clean(context.Background(), cands[0]); err == nil {
		t.Fatal("cleaned a change that is not ready")
	}

	f.pr("api", 2, "MERGED", "", green)
	write(t, filepath.Join(api.Dir(), "wip.txt"), "wip", 0o644)
	cands, _ = CleanCandidates(context.Background(), f.store)
	if cands[0].Ready || !strings.Contains(cands[0].Reason, "api: 1 uncommitted files on DEV-1") {
		t.Fatalf("uncommitted work should block: %+v", cands[0])
	}
	os.Remove(filepath.Join(api.Dir(), "wip.txt"))

	// A repo already moved off the change needs no switch back.
	run(t, proto.Dir(), "git", "switch", "--quiet", "main")
	cands, _ = CleanCandidates(context.Background(), f.store)
	cc := cands[0]
	if !cc.Ready || len(cc.Switches) != 1 || cc.Switches[0].Repo != "acme/api" || len(cc.Branches) != 2 || len(cc.Worktrees) != 0 || len(cc.Files) != 1 {
		t.Fatalf("ready candidate: %+v", cc)
	}
	if err := Clean(context.Background(), cc); err != nil {
		t.Fatal(err)
	}
	for _, l := range []*change.Leg{proto, api} {
		if got := run(t, l.Dir(), "git", "branch", "--show-current"); got != "main" {
			t.Errorf("%s on %q after clean", l.Name(), got)
		}
		if out := run(t, l.Dir(), "git", "branch", "--list", "DEV-1"); out != "" {
			t.Errorf("%s kept DEV-1", l.Name())
		}
	}
	if _, err := os.Stat(cc.Dir); !os.IsNotExist(err) {
		t.Errorf("change dir survived: %v", err)
	}
}

func TestStartLeavesDirtyRepoOnItsBranch(t *testing.T) {
	f := newFixture(t)
	proto := f.repo("proto", "module example.com/proto\n\ngo 1.26\n")
	api := f.repo("api", "module example.com/api\n\ngo 1.26\n")
	write(t, filepath.Join(api.Path, "wip.txt"), "wip", 0o644)

	c, results, err := Start(context.Background(), f.store, "DEV-2", "", []workspace.Repo{proto, api}, StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !results[0].Switched || results[1].Switched || !strings.Contains(results[1].Warning, "stayed on main: 1 uncommitted files") {
		t.Fatalf("results: %+v", results)
	}
	if got := run(t, api.Path, "git", "branch", "--show-current"); got != "main" {
		t.Errorf("dirty api switched to %q", got)
	}
	if run(t, api.Path, "git", "branch", "--list", "DEV-2") == "" {
		t.Error("branch not created in the dirty repo")
	}
	if len(c.Legs) != 2 {
		t.Errorf("legs = %d, want both recorded", len(c.Legs))
	}

	os.Remove(filepath.Join(api.Path, "wip.txt"))
	for _, r := range Switch(context.Background(), c, false) {
		if r.Err != nil {
			t.Errorf("switch %s: %v", r.Leg.Name(), r.Err)
		}
	}
	if got := run(t, api.Path, "git", "branch", "--show-current"); got != "DEV-2" {
		t.Errorf("api on %q after switch", got)
	}
	for _, r := range Switch(context.Background(), c, true) {
		if r.Err != nil || r.To != "main" {
			t.Errorf("switch to base %s: %+v", r.Leg.Name(), r)
		}
	}
}

func TestOffBranchLegsAreRefused(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	g, _ := BuildGraph(c)
	api, _ := c.Leg("api")
	run(t, api.Dir(), "git", "switch", "--quiet", "main")

	if _, err := RunChecks(context.Background(), c, api, nil); err == nil || !strings.Contains(err.Error(), "api is on main, not DEV-1") {
		t.Errorf("checks off-branch: %v", err)
	}
	res := Pin(context.Background(), c, g, true)
	if len(res) != 1 || res[0].Err == nil || !strings.Contains(res[0].Err.Error(), "api is on main") {
		t.Errorf("pin off-branch: %+v", res)
	}
	f.pr("proto", 1, "OPEN", "APPROVED", green)
	f.pr("api", 2, "OPEN", "APPROVED", green)
	var found bool
	for _, tc := range TrainPreflight(context.Background(), c, g) {
		for _, p := range tc.Problems {
			found = found || strings.Contains(p, "on main, not DEV-1: switch so it can be re-pinned")
		}
	}
	if !found {
		t.Error("train should refuse to start when a leg it must re-pin is off its branch")
	}
}

func TestWorktreeChange(t *testing.T) {
	f := newFixture(t)
	proto := f.repo("proto", "module example.com/proto\n\ngo 1.26\n")
	api := f.repo("api", "module example.com/api\n\ngo 1.26\n\nrequire example.com/proto v1.0.0\n")
	write(t, filepath.Join(api.Path, "wip.txt"), "wip", 0o644)

	c, results, err := Start(context.Background(), f.store, "DEV-3", "", []workspace.Repo{proto}, StartOptions{Worktrees: true})
	if err != nil || results[0].Err != nil {
		t.Fatalf("start: %v %+v", err, results)
	}
	if !c.Worktrees {
		t.Fatal("change not marked as using worktrees")
	}
	// Adding to a worktree change follows its mode, dirty clone or not.
	c, results, err = Start(context.Background(), f.store, "DEV-3", "", []workspace.Repo{api}, StartOptions{})
	if err != nil || results[0].Err != nil {
		t.Fatalf("add: %v %+v", err, results)
	}
	for _, l := range c.Legs {
		want := filepath.Join(f.store.Dir("DEV-3"), l.Name())
		if l.Worktree != want || l.Dir() != want {
			t.Errorf("%s worktree = %q, want %q", l.Name(), l.Worktree, want)
		}
		if got := run(t, l.Dir(), "git", "branch", "--show-current"); got != "DEV-3" {
			t.Errorf("%s worktree on %q", l.Name(), got)
		}
		if got := run(t, l.Source, "git", "branch", "--show-current"); got != "main" {
			t.Errorf("%s clone switched to %q", l.Name(), got)
		}
	}
	if _, err := os.Stat(filepath.Join(api.Path, "wip.txt")); err != nil {
		t.Error("the clone's uncommitted work was touched")
	}
	for _, r := range Switch(context.Background(), c, true) {
		if !r.Skipped || r.Err != nil {
			t.Errorf("switch should skip worktree leg %s: %+v", r.Leg.Name(), r)
		}
	}
	g, _ := BuildGraph(c)
	if g.Levels["acme/proto"] != 1 || g.Levels["acme/api"] != 2 {
		t.Errorf("levels = %v", g.Levels)
	}

	// Branch mode stays the default for new changes.
	c2, _, err := Start(context.Background(), f.store, "DEV-4", "", []workspace.Repo{proto}, StartOptions{})
	if err != nil || c2.Worktrees || c2.Legs[0].Worktree != "" {
		t.Errorf("default mode: %+v %v", c2, err)
	}
}

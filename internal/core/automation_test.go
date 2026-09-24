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
	c, results, err := Start(context.Background(), f.store, "DEV-1", "Retries", []workspace.Repo{proto, api})
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
		run(f.t, l.Worktree, "git", "push", "--quiet", "-u", "origin", "DEV-1")
	}
	return c
}

func (f *fixture) commit(l change.Leg, file, data string) {
	write(f.t, filepath.Join(l.Worktree, file), data, 0o644)
	run(f.t, l.Worktree, "git", "add", ".")
	run(f.t, l.Worktree, "git", "commit", "--quiet", "-m", "change "+file)
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
	gomod, _ := os.ReadFile(filepath.Join(api.Worktree, "go.mod"))
	if !strings.Contains(string(gomod), "// pinned example.com/proto@"+protoSHA) {
		t.Errorf("api go.mod not pinned to proto's merge commit %s:\n%s", protoSHA, gomod)
	}
	if msg := run(t, api.Worktree, "git", "log", "-1", "--format=%s"); !strings.HasPrefix(msg, "Pin example.com/proto to merged ") {
		t.Errorf("pin commit message = %q", msg)
	}
	if run(t, api.Worktree, "git", "rev-parse", "HEAD") != run(t, api.Worktree, "git", "rev-parse", "@{u}") {
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

	run(t, proto.Worktree, "git", "push", "--quiet")
	head := run(t, proto.Worktree, "git", "rev-parse", "HEAD")
	res = Pin(context.Background(), c, g, true)
	if res[0].Err != nil || !res[0].Committed || res[0].Rev != head {
		t.Fatalf("pin: %+v", res[0])
	}
	if out := run(t, api.Worktree, "git", "status", "--porcelain"); out != "" {
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
	write(t, filepath.Join(api.Worktree, "wip.txt"), "wip", 0o644)
	cands, _ = CleanCandidates(context.Background(), f.store)
	if cands[0].Ready || !strings.Contains(cands[0].Reason, "api: 1 uncommitted files") {
		t.Fatalf("uncommitted work should block: %+v", cands[0])
	}
	os.Remove(filepath.Join(api.Worktree, "wip.txt"))

	cands, _ = CleanCandidates(context.Background(), f.store)
	cc := cands[0]
	if !cc.Ready || len(cc.Worktrees) != 2 || len(cc.Branches) != 2 || len(cc.Files) != 1 {
		t.Fatalf("ready candidate: %+v", cc)
	}
	if err := Clean(context.Background(), cc); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{proto.Worktree, api.Worktree, cc.Dir} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
	if out := run(t, proto.Source, "git", "branch", "--list", "DEV-1"); out != "" {
		t.Errorf("local branch kept: %q", out)
	}
}

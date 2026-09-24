package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeGH stands in for the gh CLI: one PR per repo, numbered by repo name,
// with bodies written to <dir>/<repo>.body.
const fakeGH = `#!/bin/sh
repo=$(basename "$PWD")
echo "$repo $1 $2" >> "$FAKE_GH_DIR/calls"
case "$1 $2" in
"pr view")
  if [ -f "$FAKE_GH_DIR/$repo.json" ]; then cat "$FAKE_GH_DIR/$repo.json"; else echo "no pull requests found for branch" >&2; exit 1; fi ;;
"pr create")
  case "$repo" in proto) n=1 ;; *) n=2 ;; esac
  cat > "$FAKE_GH_DIR/$repo.body"
  url="https://github.com/acme/$repo/pull/$n"
  printf '{"number":%d,"url":"%s","state":"OPEN","body":"","statusCheckRollup":[]}' "$n" "$url" > "$FAKE_GH_DIR/$repo.json"
  echo "$url" ;;
"pr edit")
  cat > "$FAKE_GH_DIR/$repo.body" ;;
*) echo "unexpected: $*" >&2; exit 1 ;;
esac
`

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo creates root/acme/<name> with a bare origin, main pushed and origin/HEAD set.
func newRepo(t *testing.T, root, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, "acme", name)
	bare := filepath.Join(root, "..", "origins", name+".git")
	git(t, filepath.Dir(root), "init", "--quiet", "--bare", "-b", "main", bare)
	git(t, filepath.Dir(root), "init", "--quiet", "-b", "main", dir)
	for p, data := range files {
		writeFile(t, filepath.Join(dir, p), data)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "--quiet", "-m", "init")
	git(t, dir, "remote", "add", "origin", bare)
	git(t, dir, "push", "--quiet", "-u", "origin", "main")
	git(t, dir, "remote", "set-head", "origin", "main")
	return dir
}

func td(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	return out.String() + errOut.String(), code
}

func mustTD(t *testing.T, args ...string) string {
	t.Helper()
	out, code := td(t, args...)
	if code != 0 {
		t.Fatalf("td %s exited %d:\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

func TestEndToEnd(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "code")
	home := filepath.Join(root, ".tandem")
	ghDir := filepath.Join(base, "gh")
	bin := filepath.Join(base, "bin")
	writeFile(t, filepath.Join(bin, "gh"), fakeGH)
	if err := os.Chmod(filepath.Join(bin, "gh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_DIR", ghDir)
	t.Setenv("TANDEM_ROOT", root)
	t.Setenv("TANDEM_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
	t.Chdir(base)

	proto := newRepo(t, root, "proto", map[string]string{
		"go.mod":   "module example.com/proto\n\ngo 1.26\n",
		"proto.go": "package proto\n\nconst Version = 1\n",
	})
	api := newRepo(t, root, "api", map[string]string{
		"go.mod": "module example.com/api\n\ngo 1.26\n\nrequire example.com/proto v1.0.0\n",
	})

	out := mustTD(t, "start", "DEV-1", "proto", "acme/api", "--title", "Add retries")
	if !strings.Contains(out, "proto → api") {
		t.Errorf("start did not infer proto → api:\n%s", out)
	}
	wtProto, wtAPI := proto, api
	for _, repo := range []string{proto, api} {
		if got := git(t, repo, "branch", "--show-current"); got != "DEV-1" {
			t.Fatalf("%s is on %q, want DEV-1 checked out in the clone", repo, got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "DEV-1", "proto")); !os.IsNotExist(err) {
		t.Errorf("start made a worktree: %v", err)
	}
	if out := mustTD(t, "start", "DEV-1", "proto"); !strings.Contains(out, "already in DEV-1") {
		t.Errorf("re-adding a leg:\n%s", out)
	}

	writeFile(t, filepath.Join(wtProto, "retry.go"), "package proto\n\nconst Retries = 3\n")
	git(t, wtProto, "add", ".")
	git(t, wtProto, "commit", "--quiet", "-m", "retries")
	writeFile(t, filepath.Join(wtAPI, "api.go"), "package api\n")
	git(t, wtAPI, "add", ".")
	git(t, wtAPI, "commit", "--quiet", "-m", "api")
	writeFile(t, filepath.Join(wtAPI, "scratch.txt"), "wip")

	out = mustTD(t, "status", "DEV-1", "--offline")
	if !regexp.MustCompile(`1\s+proto\s+↑1 clean`).MatchString(out) || !regexp.MustCompile(`2\s+api\s+↑1 1 dirty`).MatchString(out) {
		t.Errorf("status:\n%s", out)
	}

	out = mustTD(t, "check", "DEV-1", "proto")
	if !strings.Contains(out, "✓  go build -o /dev/null ./...") || !strings.Contains(out, "✓  go test ./...") {
		t.Errorf("check:\n%s", out)
	}
	writeFile(t, filepath.Join(wtProto, "broken.go"), "package proto\n\nfunc {\n")
	if out, code := td(t, "check", "DEV-1", "proto"); code == 0 || !strings.Contains(out, "✗  go build") {
		t.Errorf("check with broken code exited %d:\n%s", code, out)
	}
	os.Remove(filepath.Join(wtProto, "broken.go"))

	out = mustTD(t, "pr", "DEV-1", "--body", "Adds retries.", "--draft")
	for _, want := range []string{"https://github.com/acme/proto/pull/1", "https://github.com/acme/api/pull/2", "1 uncommitted files left out"} {
		if !strings.Contains(out, want) {
			t.Errorf("pr output missing %q:\n%s", want, out)
		}
	}
	for _, repo := range []string{"proto", "api"} {
		body, err := os.ReadFile(filepath.Join(ghDir, repo+".body"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(body), "Adds retries.") || !strings.Contains(string(body), "1. acme/proto#1\n2. acme/api#2\n") {
			t.Errorf("%s body:\n%s", repo, body)
		}
	}
	if git(t, proto, "ls-remote", "origin", "DEV-1") == "" {
		t.Error("DEV-1 was not pushed to proto's origin")
	}

	mustTD(t, "pr", "DEV-1")
	calls, _ := os.ReadFile(filepath.Join(ghDir, "calls"))
	if n := strings.Count(string(calls), "pr create"); n != 2 {
		t.Errorf("second td pr created PRs again: %d creates\n%s", n, calls)
	}

	// Someone else lands a commit on main.
	other := filepath.Join(base, "other")
	git(t, base, "clone", "--quiet", filepath.Join(base, "origins", "proto.git"), other)
	writeFile(t, filepath.Join(other, "upstream.go"), "package proto\n")
	git(t, other, "add", ".")
	git(t, other, "commit", "--quiet", "-m", "upstream")
	git(t, other, "push", "--quiet", "origin", "main")
	out, _ = td(t, "sync", "DEV-1")
	if !strings.Contains(out, "rebased onto origin/main (1 new commits) · already pushed") {
		t.Errorf("sync proto:\n%s", out)
	}
	if !strings.Contains(out, "skipped: 1 uncommitted files") {
		t.Errorf("sync should skip dirty api:\n%s", out)
	}

	// Inside a leg's repo, the change is found without naming it.
	t.Chdir(filepath.Join(wtAPI))
	if out := mustTD(t, "path", "proto"); strings.TrimSpace(out) != wtProto {
		t.Errorf("path from inside a leg's repo = %q", out)
	}

	out, code := td(t, "switch", "--base")
	if code == 0 || !regexp.MustCompile(`proto\s+on main`).MatchString(out) || !regexp.MustCompile(`api\s+stayed put: 1 uncommitted files`).MatchString(out) {
		t.Errorf("switch --base exited %d:\n%s", code, out)
	}
	if got := git(t, proto, "branch", "--show-current"); got != "main" {
		t.Errorf("proto on %q after switch --base", got)
	}
	if got := git(t, api, "branch", "--show-current"); got != "DEV-1" {
		t.Errorf("dirty api was switched to %q", got)
	}
	out = mustTD(t, "status", "DEV-1", "--offline")
	if !regexp.MustCompile(`proto\s+↑1 · on main`).MatchString(out) {
		t.Errorf("status should count DEV-1 while proto is on main:\n%s", out)
	}
	if out, code := td(t, "check", "DEV-1", "proto"); code == 0 || !strings.Contains(out, "proto is on main, not DEV-1: switch to it first") {
		t.Errorf("check off-branch exited %d:\n%s", code, out)
	}
	os.Remove(filepath.Join(api, "scratch.txt"))
	mustTD(t, "switch", "DEV-1")
	if got := git(t, proto, "branch", "--show-current"); got != "DEV-1" {
		t.Errorf("proto on %q after switch", got)
	}
}

// trainGH extends the fake with merging: views substitute __HEAD__ with the
// pushed commit, and pr merge flips the PR to MERGED at that commit.
const trainGH = `#!/bin/sh
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

func TestMergeAndCleanCLI(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "code")
	home := filepath.Join(root, ".tandem")
	ghDir := filepath.Join(base, "gh")
	bin := filepath.Join(base, "bin")
	writeFile(t, filepath.Join(bin, "gh"), trainGH)
	writeFile(t, filepath.Join(bin, "go"), "#!/bin/sh\n[ \"$1\" = get ] && echo \"// pinned $2\" >> go.mod\n")
	for _, b := range []string{"gh", "go"} {
		if err := os.Chmod(filepath.Join(bin, b), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_DIR", ghDir)
	t.Setenv("TANDEM_ROOT", root)
	t.Setenv("TANDEM_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
	t.Chdir(base)

	newRepo(t, root, "proto", map[string]string{"go.mod": "module example.com/proto\n\ngo 1.26\n"})
	newRepo(t, root, "api", map[string]string{"go.mod": "module example.com/api\n\ngo 1.26\n\nrequire example.com/proto v1.0.0\n"})
	mustTD(t, "start", "DEV-7", "proto", "api", "--title", "Retries")
	for _, leg := range []string{"proto", "api"} {
		wt := filepath.Join(root, "acme", leg)
		writeFile(t, filepath.Join(wt, "work.txt"), leg)
		git(t, wt, "add", ".")
		git(t, wt, "commit", "--quiet", "-m", "work")
		git(t, wt, "push", "--quiet", "-u", "origin", "DEV-7")
	}
	pr := func(repo string, n int, review, state string) {
		writeFile(t, filepath.Join(ghDir, repo+".json"), fmt.Sprintf(
			`{"number":%d,"url":"https://github.com/acme/%s/pull/%d","state":"%s","body":"","isDraft":false,"reviewDecision":"%s","headRefOid":"__HEAD__","statusCheckRollup":[{"name":"test","status":"COMPLETED","conclusion":"SUCCESS"}],"mergeCommit":null}`,
			n, repo, n, state, review))
	}

	out := mustTD(t, "pin", "DEV-7")
	if !regexp.MustCompile(`api ← example.com/proto\s+pinned to \w{12}, committed`).MatchString(out) {
		t.Errorf("pin:\n%s", out)
	}
	git(t, filepath.Join(root, "acme", "api"), "push", "--quiet")

	pr("proto", 1, "CHANGES_REQUESTED", "OPEN")
	if out, code := td(t, "merge", "DEV-7", "--yes"); code == 0 || !strings.Contains(out, "changes requested") || !strings.Contains(out, "2 of 2 legs blocked") {
		t.Errorf("blocked train exited %d:\n%s", code, out)
	}
	pr("proto", 1, "APPROVED", "OPEN")
	pr("api", 2, "APPROVED", "OPEN")
	if out, code := td(t, "merge", "DEV-7"); code == 0 || !strings.Contains(out, "pass --yes") {
		t.Errorf("merge without a terminal or --yes exited %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(ghDir, "merged")); err == nil {
		t.Fatal("merged without confirmation")
	}

	// The train runs with its real 20s poll, so pins must not leave anything to wait on.
	out = mustTD(t, "merge", "DEV-7", "--yes")
	for _, want := range []string{"proto", "merging", "api", "pinned", "every leg merged"} {
		if !strings.Contains(out, want) {
			t.Errorf("merge output missing %q:\n%s", want, out)
		}
	}
	merged, _ := os.ReadFile(filepath.Join(ghDir, "merged"))
	if string(merged) != "proto\napi\n" {
		t.Errorf("merge order = %q", merged)
	}

	out = mustTD(t, "clean", "--yes")
	for _, want := range []string{"switch to main   " + filepath.Join(root, "acme", "proto"), "delete branch    DEV-7", "DEV-7 cleaned"} {
		if !strings.Contains(out, want) {
			t.Errorf("clean output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "DEV-7")); !os.IsNotExist(err) {
		t.Errorf("change dir survived clean: %v", err)
	}
	for _, leg := range []string{"proto", "api"} {
		repo := filepath.Join(root, "acme", leg)
		if got := git(t, repo, "branch", "--show-current"); got != "main" {
			t.Errorf("%s left on %q after clean", leg, got)
		}
		if out := git(t, repo, "branch", "--list", "DEV-7"); out != "" {
			t.Errorf("%s kept branch DEV-7", leg)
		}
	}
}

// A change started without a repo it turns out to need: adding it gives it the
// branch and slots it into the merge order.
func TestAddRepoToChange(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "code")
	t.Setenv("TANDEM_ROOT", root)
	t.Setenv("TANDEM_HOME", filepath.Join(root, ".tandem"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
	t.Chdir(base)

	newRepo(t, root, "proto", map[string]string{"go.mod": "module example.com/proto\n\ngo 1.26\n"})
	grpc := newRepo(t, root, "grpc", map[string]string{"go.mod": "module example.com/grpc\n\ngo 1.26\n"})
	api := newRepo(t, root, "api", map[string]string{
		"go.mod": "module example.com/api\n\ngo 1.26\n\nrequire (\n\texample.com/proto v1.0.0\n\texample.com/grpc v1.0.0\n)\n",
	})
	mustTD(t, "start", "DEV-9", "proto", "api", "--title", "Streaming")
	levels := func() string {
		t.Helper()
		out := mustTD(t, "status", "DEV-9", "--offline")
		var rows []string
		for _, line := range strings.Split(out, "\n")[2:] {
			if f := strings.Fields(line); len(f) >= 2 {
				rows = append(rows, f[0]+" "+f[1])
			}
		}
		return strings.Join(rows, ", ")
	}
	if got := levels(); got != "1 proto, 2 api" {
		t.Fatalf("before: %s", got)
	}

	out := mustTD(t, "add", "DEV-9", "grpc")
	for _, want := range []string{"acme/grpc", "branch DEV-9, checked out", "grpc → api", "td pr"} {
		if !strings.Contains(out, want) {
			t.Errorf("add output missing %q:\n%s", want, out)
		}
	}
	if got := git(t, grpc, "branch", "--show-current"); got != "DEV-9" {
		t.Errorf("grpc on %q", got)
	}
	if got := levels(); got != "1 grpc, 1 proto, 2 api" {
		t.Errorf("after add: %s", got)
	}
	if out := mustTD(t, "add", "DEV-9", "grpc"); !strings.Contains(out, "already in DEV-9") {
		t.Errorf("adding twice:\n%s", out)
	}

	// A dependency no manifest shows yet is declared, and can be taken back.
	mustTD(t, "link", "DEV-9", "grpc", "proto")
	if got := levels(); got != "1 grpc, 2 proto, 3 api" {
		t.Errorf("after link: %s", got)
	}
	mustTD(t, "unlink", "DEV-9", "grpc", "proto")
	if got := levels(); got != "1 grpc, 1 proto, 2 api" {
		t.Errorf("after unlink: %s", got)
	}
	if out, code := td(t, "unlink", "DEV-9", "grpc", "api"); code == 0 || !strings.Contains(out, "not a declared edge") {
		t.Errorf("unlinking an inferred edge exited %d:\n%s", code, out)
	}

	// Inside the api repo the change is found without naming it.
	t.Chdir(api)
	mustTD(t, "link", "grpc", "proto")
	out = mustTD(t, "remove", "grpc")
	if !strings.Contains(out, "acme/grpc removed from DEV-9") || !strings.Contains(out, "branch DEV-9 is left in "+grpc) {
		t.Errorf("remove:\n%s", out)
	}
	if got := levels(); got != "1 proto, 2 api" {
		t.Errorf("after remove: %s", got)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".tandem", "DEV-9", "change.json"))
	if strings.Contains(string(data), "acme/grpc") {
		t.Errorf("removed leg or its edge still recorded:\n%s", data)
	}
	if git(t, grpc, "branch", "--list", "DEV-9") == "" {
		t.Error("remove deleted grpc's branch")
	}
}

func TestStartWithWorktrees(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "code")
	home := filepath.Join(root, ".tandem")
	t.Setenv("TANDEM_ROOT", root)
	t.Setenv("TANDEM_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
	t.Chdir(base)
	proto := newRepo(t, root, "proto", map[string]string{"go.mod": "module example.com/proto\n\ngo 1.26\n"})
	newRepo(t, root, "api", map[string]string{"go.mod": "module example.com/api\n\ngo 1.26\n"})

	wt := filepath.Join(home, "DEV-W", "proto")
	out := mustTD(t, "start", "DEV-W", "proto", "--worktree")
	if !strings.Contains(out, "worktree "+wt) {
		t.Errorf("start --worktree:\n%s", out)
	}
	if got := git(t, proto, "branch", "--show-current"); got != "main" {
		t.Errorf("clone switched to %q", got)
	}
	if got := strings.TrimSpace(mustTD(t, "path", "DEV-W", "proto")); got != wt {
		t.Errorf("path = %q", got)
	}
	if out := mustTD(t, "add", "DEV-W", "api"); !strings.Contains(out, "worktree "+filepath.Join(home, "DEV-W", "api")) {
		t.Errorf("add to a worktree change:\n%s", out)
	}
	if out := mustTD(t, "switch", "DEV-W", "--base"); !strings.Contains(out, "worktree, always on DEV-W") {
		t.Errorf("switch on a worktree change:\n%s", out)
	}

	mustTD(t, "start", "DEV-B", "proto")
	if out := mustTD(t, "start", "DEV-B", "api", "--worktree"); !strings.Contains(out, "--worktree only applies when a change is created") {
		t.Errorf("--worktree on a branch change:\n%s", out)
	}
}

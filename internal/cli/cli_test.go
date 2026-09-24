package cli

import (
	"bytes"
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
	newRepo(t, root, "api", map[string]string{
		"go.mod": "module example.com/api\n\ngo 1.26\n\nrequire example.com/proto v1.0.0\n",
	})

	out := mustTD(t, "start", "DEV-1", "proto", "acme/api", "--title", "Add retries")
	if !strings.Contains(out, "proto → api") {
		t.Errorf("start did not infer proto → api:\n%s", out)
	}
	wtProto := filepath.Join(home, "DEV-1", "proto")
	wtAPI := filepath.Join(home, "DEV-1", "api")
	if got := git(t, wtAPI, "branch", "--show-current"); got != "DEV-1" {
		t.Fatalf("api worktree on %q", got)
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

	writeFile(t, filepath.Join(proto, "upstream.go"), "package proto\n")
	git(t, proto, "add", ".")
	git(t, proto, "commit", "--quiet", "-m", "upstream")
	git(t, proto, "push", "--quiet", "origin", "main")
	out, _ = td(t, "sync", "DEV-1")
	if !strings.Contains(out, "rebased onto origin/main (1 new commits) · already pushed") {
		t.Errorf("sync proto:\n%s", out)
	}
	if !strings.Contains(out, "skipped: 1 uncommitted files") {
		t.Errorf("sync should skip dirty api:\n%s", out)
	}

	t.Chdir(wtAPI)
	if out := mustTD(t, "path", "proto"); strings.TrimSpace(out) != wtProto {
		t.Errorf("path from inside a worktree = %q", out)
	}
}

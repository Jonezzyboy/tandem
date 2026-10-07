package testrun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	t    *testing.T
	base string
}

func newFixture(t *testing.T) *fixture {
	base := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "Test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}
	return &fixture{t: t, base: base}
}

// repo makes origin acme/<name> with a main commit and a DEV-1 branch pushed
// by a developer, and returns a tester's clone of it.
func (f *fixture) repo(name string) (origin, dev, tester string) {
	origin = filepath.Join(f.base, "origins", name+".git")
	dev = filepath.Join(f.base, "dev", name)
	tester = filepath.Join(f.base, "code", "acme", name)
	git(f.t, f.base, "init", "--quiet", "--bare", "-b", "main", origin)
	git(f.t, f.base, "clone", "--quiet", origin, dev)
	write(f.t, filepath.Join(dev, "app.txt"), "v1\n")
	git(f.t, dev, "add", ".")
	git(f.t, dev, "commit", "--quiet", "-m", "init")
	git(f.t, dev, "push", "--quiet", "origin", "main")
	git(f.t, f.base, "clone", "--quiet", origin, tester)
	git(f.t, dev, "switch", "--quiet", "-c", "DEV-1")
	f.commit(dev, "feature.txt", "feature\n", "feature")
	git(f.t, dev, "push", "--quiet", "-u", "origin", "DEV-1")
	return origin, dev, tester
}

func (f *fixture) commit(dir, file, data, msg string) {
	write(f.t, filepath.Join(dir, file), data)
	git(f.t, dir, "add", ".")
	git(f.t, dir, "commit", "--quiet", "-m", msg)
}

func TestTestingRoundTrip(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, apiDev, api := f.repo("api")
	webOrigin, webDev, webClone := f.repo("web")
	if err := os.RemoveAll(webClone); err != nil {
		t.Fatal(err)
	}
	// The tester has uncommitted work in api, and hasn't cloned web.
	write(t, filepath.Join(api, "app.txt"), "my local tweak\n")

	var progress []string
	cloneRoot := filepath.Join(f.base, "code")
	s, steps := Begin(ctx, "DEV-1", "Feature", "https://x/browse/DEV-1",
		[]Target{{Name: "acme/api", Dir: api}, {Name: "acme/web"}},
		Options{
			SetAside: true, CloneRoot: cloneRoot,
			Clone: func(ctx context.Context, repo, dir string) error {
				return exec.CommandContext(ctx, "git", "clone", "--quiet", webOrigin, dir).Run()
			},
			OnStep: func(s Step) { progress = append(progress, s.Repo+": "+s.Message) },
		})
	for _, st := range steps {
		if !st.OK {
			t.Fatalf("begin: %+v", steps)
		}
	}
	if !strings.Contains(steps[0].Message, "set aside 1 uncommitted files") || !strings.Contains(steps[1].Message, "cloned") || len(s.Repos) != 2 {
		t.Fatalf("steps %+v, session %+v", steps, s)
	}
	web := filepath.Join(cloneRoot, "acme", "web")
	for _, dir := range []string{api, web} {
		if got := git(t, dir, "branch", "--show-current"); got != "DEV-1" {
			t.Errorf("%s on %q", dir, got)
		}
	}
	if len(progress) < 4 {
		t.Errorf("progress = %q", progress)
	}

	// The developer pushes more; the tester sees it and pulls.
	f.commit(apiDev, "feature.txt", "feature v2\n", "Handle zero pence")
	git(t, apiDev, "push", "--quiet")
	st := Status(ctx, s)
	if !st[0].OnBranch || st[0].Behind != 1 || len(st[0].New) != 1 || st[0].New[0] != "Handle zero pence" || st[1].Behind != 0 {
		t.Fatalf("status = %+v", st)
	}
	if pulled := Pull(ctx, s); !pulled[0].OK || !strings.HasPrefix(pulled[0].Message, "pulled to") {
		t.Fatalf("pull = %+v", pulled)
	}
	if st := Status(ctx, s); st[0].Behind != 0 {
		t.Errorf("still behind after pull: %+v", st[0])
	}

	// Main moves on while testing; ending pulls it and restores the work.
	git(t, webDev, "switch", "--quiet", "main")
	f.commit(webDev, "app.txt", "v2\n", "main moves")
	git(t, webDev, "push", "--quiet", "origin", "main")
	ended, left := End(ctx, s)
	if len(left) != 0 || !strings.Contains(ended[0].Message, "your uncommitted work is back") || !strings.Contains(ended[1].Message, "pulled 1 new commits") {
		t.Fatalf("end = %+v, left %+v", ended, left)
	}
	for _, dir := range []string{api, web} {
		if got := git(t, dir, "branch", "--show-current"); got != "main" {
			t.Errorf("%s on %q after end", dir, got)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(api, "app.txt")); string(data) != "my local tweak\n" {
		t.Errorf("api work = %q", data)
	}
}

func TestBeginRefusesWithoutSetAsideAndEndKeepsDirtyRepos(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _, api := f.repo("api")
	write(t, filepath.Join(api, "app.txt"), "tweak\n")
	s, steps := Begin(ctx, "DEV-1", "", "", []Target{{Name: "acme/api", Dir: api}, {Name: "acme/gone"}}, Options{})
	if steps[0].OK || !strings.Contains(steps[0].Message, "1 uncommitted files") || steps[1].OK || len(s.Repos) != 0 {
		t.Fatalf("steps %+v", steps)
	}
	if got := git(t, api, "branch", "--show-current"); got != "main" {
		t.Errorf("api switched to %q", got)
	}

	git(t, api, "checkout", "--", "app.txt")
	s, _ = Begin(ctx, "DEV-1", "", "", []Target{{Name: "acme/api", Dir: api}}, Options{})
	write(t, filepath.Join(api, "feature.txt"), "edited while testing\n")
	if ended, left := End(ctx, s); ended[0].OK || len(left) != 1 || !strings.Contains(ended[0].Message, "uncommitted files from testing") {
		t.Fatalf("end = %+v left %+v", ended, left)
	}
}

func TestStoreKeepsRecentHistory(t *testing.T) {
	st := Store{Path: filepath.Join(t.TempDir(), "tester.json")}
	if got, err := st.Load(); err != nil || got.Current != nil {
		t.Fatalf("empty load = %+v %v", got, err)
	}
	var in State
	in.Current = &Session{Key: "DEV-1"}
	for range historyLimit + 5 {
		in.History = append(in.History, Record{Key: "DEV-1"})
	}
	if err := st.Save(in); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil || got.Current.Key != "DEV-1" || len(got.History) != historyLimit {
		t.Errorf("load = %+v (%d history) %v", got.Current, len(got.History), err)
	}
}

func TestBeginUsesEachRepoBranch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, dev, api := f.repo("api")
	git(t, dev, "switch", "--quiet", "-c", "DEV-1-price-fix")
	f.commit(dev, "fix.txt", "fix\n", "Price fix")
	git(t, dev, "push", "--quiet", "-u", "origin", "DEV-1-price-fix")

	s, steps := Begin(ctx, "DEV-1", "", "", []Target{{Name: "acme/api", Dir: api, Branch: "DEV-1-price-fix"}}, Options{})
	if !steps[0].OK || !strings.Contains(steps[0].Message, "switched to DEV-1-price-fix") {
		t.Fatalf("steps = %+v", steps)
	}
	if got := git(t, api, "branch", "--show-current"); got != "DEV-1-price-fix" {
		t.Errorf("on %q", got)
	}
	if st := Status(ctx, s); !st[0].OnBranch || st[0].Branch != "DEV-1-price-fix" {
		t.Errorf("status = %+v", st[0])
	}
	if _, left := End(ctx, s); len(left) != 0 {
		t.Errorf("left = %+v", left)
	}
}

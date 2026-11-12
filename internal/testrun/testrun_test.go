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

	// The record keeps what was tested, so a retest can list what's new since.
	s.Checks = []Check{{Text: "zero pence", Done: true}, {Text: "refunds"}}
	rec := s.Record()
	if len(rec.Repos) != 2 || rec.Repos[0].SHA == "" || rec.Repos[0].Branch != "DEV-1" || rec.Checks != 2 || rec.Checked != 1 {
		t.Fatalf("record = %+v", rec)
	}
	f.commit(apiDev, "feature.txt", "feature v3\n", "Round prices")
	git(t, apiDev, "push", "--quiet")
	since, err := Since(ctx, api, "DEV-1", rec.Repos[0].SHA)
	if err != nil || len(since) != 1 || since[0].Subject != "Round prices" || since[0].At.IsZero() {
		t.Errorf("since = %+v, %v", since, err)
	}
	if _, err := Since(ctx, api, "DEV-1", "0123456789abcdef0123456789abcdef01234567"); err == nil {
		t.Error("since an unknown commit: no error")
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

func TestPullFollowsARewrittenBranch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, dev, api := f.repo("api")
	s, steps := Begin(ctx, "DEV-1", "", "", []Target{{Name: "acme/api", Dir: api}}, Options{})
	if !steps[0].OK {
		t.Fatalf("begin: %+v", steps)
	}

	git(t, dev, "commit", "--quiet", "--amend", "-m", "feature, reworded")
	git(t, dev, "push", "--quiet", "--force")
	pulled := Pull(ctx, s)
	if !pulled[0].OK || !strings.HasPrefix(pulled[0].Message, "origin was rewritten") {
		t.Fatalf("pull = %+v", pulled)
	}
	if got, want := git(t, api, "rev-parse", "HEAD"), git(t, dev, "rev-parse", "HEAD"); got != want {
		t.Errorf("HEAD %s, want %s", got, want)
	}

	// A commit made here is never thrown away.
	f.commit(api, "mine.txt", "mine\n", "tester's own")
	git(t, dev, "commit", "--quiet", "--amend", "-m", "feature, again")
	git(t, dev, "push", "--quiet", "--force")
	if pulled := Pull(ctx, s); pulled[0].OK || !strings.Contains(pulled[0].Message, "has commits origin doesn't") {
		t.Fatalf("pull = %+v", pulled)
	}
	if got := git(t, api, "log", "-1", "--format=%s"); got != "tester's own" {
		t.Errorf("HEAD is %q", got)
	}
}

func TestLinkGoUsesLocalUpstreamUntilTestingEnds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, protoDev, proto := f.repo("proto")
	_, apiDev, api := f.repo("api")
	_, svcDev, svc := f.repo("svc")
	_, webDev, web := f.repo("web")
	const apiMod = "module example.com/api\n\ngo 1.21\n\nrequire example.com/proto v1.0.0\n"
	f.commit(protoDev, "go.mod", "module example.com/proto\n\ngo 1.22\n", "module")
	f.commit(apiDev, "go.mod", apiMod, "require proto")
	f.commit(apiDev, "tools/go.mod", "module example.com/api/tools\n\ngo 1.21\n", "tools module")
	f.commit(svcDev, "go.mod", "module example.com/svc\n\ngo 1.22\n\nrequire example.com/proto v1.0.0\n", "require proto")
	f.commit(svcDev, "vendor/modules.txt", "# example.com/proto v1.0.0\n", "vendor")
	f.commit(webDev, "go.mod", "module example.com/web\n\ngo 1.22\n\nrequire example.com/proto v1.0.0\n", "require proto")
	for _, d := range []string{protoDev, apiDev, svcDev, webDev} {
		git(t, d, "push", "--quiet", "origin", "DEV-1")
	}
	targets := []Target{{Name: "acme/proto", Dir: proto}, {Name: "acme/api", Dir: api}, {Name: "acme/svc", Dir: svc}, {Name: "acme/web", Dir: web}}
	s, steps := Begin(ctx, "DEV-1", "", "", targets, Options{})
	for _, st := range steps {
		if !st.OK {
			t.Fatalf("%s: %s", st.Repo, st.Message)
		}
	}
	const own = "go 1.22\n\nuse .\n"
	write(t, filepath.Join(web, "go.work"), own)

	notes := LinkGo(ctx, s)
	if notes["acme/api"] != "using local proto via go.work" || !strings.Contains(notes["acme/svc"], "vendors its modules") ||
		!strings.Contains(notes["acme/web"], "already has a go.work") || notes["acme/proto"] != "" {
		t.Fatalf("notes = %q", notes)
	}
	work, err := os.ReadFile(filepath.Join(api, "go.work"))
	if err != nil || !strings.Contains(string(work), "go 1.22\n") || !strings.Contains(string(work), "\t.\n\t../proto\n\ttools\n") {
		t.Fatalf("api go.work = %q, %v", work, err)
	}
	if _, err := os.Stat(filepath.Join(svc, "go.work")); err == nil {
		t.Error("svc vendors, so it should get no go.work")
	}
	want, _ := filepath.EvalSymlinks(proto)
	for _, dir := range []string{api, filepath.Join(api, "tools")} {
		cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "example.com/proto")
		cmd.Dir, cmd.Env = dir, append(os.Environ(), "GOPROXY=off", "GOFLAGS=")
		if out, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != want {
			t.Errorf("in %s go resolves proto to %q, %v; want %s", dir, out, err, want)
		}
	}
	if n := uncommitted(ctx, api); n != 0 {
		t.Errorf("api has %d uncommitted files", n)
	}

	// A go command in workspace mode rewrote go.mod and left go.work.sum.
	write(t, filepath.Join(api, "go.mod"), apiMod+"// rewritten\n")
	write(t, filepath.Join(api, "go.work.sum"), "")

	// An edit that blocks going back keeps api under test, still linked.
	write(t, filepath.Join(api, "app.txt"), "edited\n")
	if _, left := End(ctx, s); len(left) != 1 || left[0].Name != "acme/api" {
		t.Fatalf("left = %+v, want only api", left)
	}
	if _, err := os.Stat(filepath.Join(api, "go.work")); err != nil {
		t.Fatal("api lost its go.work while still under test")
	}
	git(t, api, "checkout", "--", "app.txt")
	s.Repos = []Repo{s.Repos[1]}

	steps, left := End(ctx, s)
	if len(left) != 0 {
		t.Fatalf("end: %+v", steps)
	}
	if rec := s.Record(); len(rec.Repos) != 4 {
		t.Errorf("record after a retried end lists %+v, want every repo tested", rec.Repos)
	}
	for _, st := range steps {
		if st.Repo == "acme/api" && !strings.Contains(st.Message, "put back go.mod/go.sum in the root") {
			t.Errorf("api end step = %q, want the rewritten go.mod put back", st.Message)
		}
	}
	for _, f := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(filepath.Join(api, f)); err == nil {
			t.Errorf("api/%s left behind", f)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(web, "go.work")); string(got) != own {
		t.Errorf("web's own go.work = %q, want it untouched", got)
	}
}

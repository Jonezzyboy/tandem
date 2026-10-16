package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
)

func TestLegState(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("a.go", "package a\n")
	git("add", ".")
	git("commit", "-qm", "one")
	ctx := context.Background()
	clean := legState(ctx, dir)
	if clean == "" || legState(ctx, dir) != clean {
		t.Fatalf("state isn't stable: %q", clean)
	}
	write("a.go", "package a\n// edit\n")
	edited := legState(ctx, dir)
	if edited == clean {
		t.Error("an uncommitted edit didn't change the state")
	}
	write("new.go", "package a\n")
	untracked := legState(ctx, dir)
	write("new.go", "package a\n// more\n")
	if untracked == edited || legState(ctx, dir) == untracked {
		t.Error("an untracked file or its contents didn't change the state")
	}
	if legState(ctx, t.TempDir()) != "" {
		t.Error("a non-repo has a state")
	}
}

func TestPassedChecks(t *testing.T) {
	a := NewApp(changeStoreFor(t), nil)
	p := a.passedChecks("ABC-1")
	p.record("api", "s1", true)
	if !a.passedChecks("ABC-1").same("api", "s1") {
		t.Error("a passing state wasn't kept")
	}
	p.record("api", "s2", false)
	if a.passedChecks("ABC-1").same("api", "s1") || a.passedChecks("ABC-1").same("api", "s2") {
		t.Error("a failing run left a state to skip on")
	}
	if p.same("api", "") {
		t.Error("an unknown state matched")
	}
}

func changeStoreFor(t *testing.T) change.Store {
	t.Helper()
	s := change.Store{Home: t.TempDir()}
	if err := s.Save(&change.Change{ID: "ABC-1", Branch: "ABC-1"}); err != nil {
		t.Fatal(err)
	}
	return s
}

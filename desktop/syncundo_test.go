package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonezzyboy/tandem/internal/change"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t", "-c", "init.defaultBranch=main"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-qm", name)
}

// syncFixture is a clone on branch X-1, one commit ahead of main, with
// origin/main a commit ahead of where X-1 started.
func syncFixture(t *testing.T) (*App, string) {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	clone := filepath.Join(root, "clone")
	os.MkdirAll(seed, 0o755)
	gitIn(t, root, "init", "-q", "--bare", origin)
	gitIn(t, seed, "init", "-q")
	commitFile(t, seed, "a.txt", "a")
	gitIn(t, seed, "remote", "add", "origin", origin)
	gitIn(t, seed, "push", "-q", "origin", "main")
	gitIn(t, root, "clone", "-q", origin, clone)
	gitIn(t, clone, "switch", "-qc", "X-1")
	commitFile(t, clone, "b.txt", "b")
	commitFile(t, seed, "c.txt", "c")
	gitIn(t, seed, "push", "-q", "origin", "main")

	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	a.ctx = context.Background()
	c := &change.Change{ID: "X-1", Branch: "X-1", Legs: []change.Leg{{Repo: "acme/api", Source: clone, Base: "main", BaseRef: "origin/main"}}}
	if err := a.store.Save(c); err != nil {
		t.Fatal(err)
	}
	return a, clone
}

func TestSyncPlanAndUndo(t *testing.T) {
	a, clone := syncFixture(t)
	plan, err := a.SyncPlan("X-1")
	if err != nil || len(plan) != 1 || plan[0].Skip != "" || strings.Join(plan[0].Commands, "; ") != "git fetch origin; git rebase origin/main" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	before := gitIn(t, clone, "rev-parse", "HEAD")
	if res, err := a.Sync("X-1"); err != nil || !res[0].OK {
		t.Fatalf("sync = %+v, %v", res, err)
	}
	if gitIn(t, clone, "rev-parse", "HEAD") == before {
		t.Fatal("sync didn't rebase")
	}
	if legs := a.CanUndoSync("X-1"); len(legs) != 1 || legs[0] != "api" {
		t.Fatalf("CanUndoSync = %q", legs)
	}
	res, err := a.UndoSync("X-1")
	if err != nil || len(res) != 1 || !res[0].OK {
		t.Fatalf("undo = %+v, %v", res, err)
	}
	if got := gitIn(t, clone, "rev-parse", "HEAD"); got != before {
		t.Errorf("after undo HEAD = %s, want %s", got, before)
	}
	if len(a.CanUndoSync("X-1")) != 0 {
		t.Error("undo is still offered after undoing")
	}
}

func TestUndoSyncLeavesMovedLegs(t *testing.T) {
	a, clone := syncFixture(t)
	if _, err := a.Sync("X-1"); err != nil {
		t.Fatal(err)
	}
	commitFile(t, clone, "d.txt", "d")
	moved := gitIn(t, clone, "rev-parse", "HEAD")
	res, err := a.UndoSync("X-1")
	if err != nil || len(res) != 1 || res[0].OK || !strings.Contains(res[0].Message, "new commits") {
		t.Fatalf("undo = %+v, %v", res, err)
	}
	if gitIn(t, clone, "rev-parse", "HEAD") != moved {
		t.Error("undo moved a leg with new commits")
	}
}

package core

import (
	"context"
	"slices"
	"testing"

	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

func TestSetBranchRenamesOrRepoints(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	ctx := context.Background()
	proto, api := &c.Legs[0], &c.Legs[1]
	proto.PR, api.PR = 5, 7
	f.mergedPR("proto", 5, "abc")
	// proto already has DEV-2 on origin only; api has no DEV-2 anywhere.
	run(t, proto.Dir(), "git", "push", "--quiet", "origin", "main:DEV-2")
	run(t, proto.Dir(), "git", "fetch", "--quiet", "origin")

	if err := SetBranch(ctx, c, "DEV-2"); err != nil {
		t.Fatal(err)
	}
	if c.Branch != "DEV-2" || proto.PR != 5 || api.PR != 0 {
		t.Fatalf("branch %q, proto PR %d, api PR %d; want merged PR kept, open one cleared", c.Branch, proto.PR, api.PR)
	}
	if !gitx.RefExists(ctx, proto.Dir(), "refs/heads/DEV-1") || !gitx.RefExists(ctx, proto.Dir(), "refs/heads/DEV-2") {
		t.Fatal("proto should keep DEV-1 and track DEV-2")
	}
	if gitx.RefExists(ctx, api.Dir(), "refs/heads/DEV-1") || gitx.CurrentBranch(ctx, api.Dir()) != "DEV-2" {
		t.Fatal("api's DEV-1 should be renamed to DEV-2 and stay checked out")
	}

	if err := SetBranch(ctx, c, "bad..name"); err == nil {
		t.Fatal("want invalid name refused")
	}
	if err := SetBranch(ctx, c, "DEV-1"); err != nil {
		t.Fatal(err)
	}
	if c.Branch != "DEV-1" || gitx.CurrentBranch(ctx, api.Dir()) != "DEV-2" {
		t.Fatal("api should repoint to its pushed origin/DEV-1 without moving its checkout")
	}
}

func TestSetBranchUndoesStepsWhenALegFails(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	proto := f.repo("proto", "module example.com/proto\n\ngo 1.26\n")
	api := f.repo("api", "module example.com/api\n\ngo 1.26\n")
	c, _, err := Start(ctx, f.store, "DEV-1", "Retries", []workspace.Repo{proto, api}, StartOptions{Worktrees: true})
	if err != nil {
		t.Fatal(err)
	}
	first, second := c.Legs[0], c.Legs[1]
	// second's clone has DEV-3 checked out, so its worktree can't switch to it;
	// first has no DEV-3 and gets its branch renamed before that fails.
	run(t, second.Source, "git", "switch", "--quiet", "-c", "DEV-3")

	if err := SetBranch(ctx, c, "DEV-3"); err == nil {
		t.Fatal("want the switch in " + second.Name() + " to fail")
	}
	if c.Branch != "DEV-1" {
		t.Fatalf("branch %q after a failure", c.Branch)
	}
	if !gitx.RefExists(ctx, first.Dir(), "refs/heads/DEV-1") || gitx.RefExists(ctx, first.Dir(), "refs/heads/DEV-3") {
		t.Fatal(first.Name() + "'s rename to DEV-3 should be undone")
	}
}

func TestKeyBranches(t *testing.T) {
	f := newFixture(t)
	proto := f.repo("proto", "module example.com/proto\n\ngo 1.26\n")
	api := f.repo("api", "module example.com/api\n\ngo 1.26\n")
	for _, b := range []string{"DEV-3586", "DEV-3586-license-key-card", "feature/dev-3586-more", "DEV-35860-other", "DEV-1"} {
		run(t, proto.Path, "git", "branch", b)
	}
	run(t, api.Path, "git", "push", "--quiet", "origin", "main:DEV-3586-license-key-card")
	run(t, api.Path, "git", "fetch", "--quiet", "origin")

	got := KeyBranches(context.Background(), []workspace.Repo{proto, api}, "DEV-3586")
	if want := []string{"DEV-3586-license-key-card", "feature/dev-3586-more"}; !slices.Equal(got, want) {
		t.Errorf("KeyBranches = %v, want %v", got, want)
	}
}

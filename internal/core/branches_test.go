package core

import (
	"context"
	"testing"

	"github.com/jonezzyboy/tandem/internal/gitx"
)

func TestSetBranchRenamesOrRepoints(t *testing.T) {
	f := newFixture(t)
	c := f.change()
	ctx := context.Background()
	proto, api := &c.Legs[0], &c.Legs[1]
	api.PR = 7
	// proto already has DEV-2 on origin only; api has no DEV-2 anywhere.
	run(t, proto.Dir(), "git", "push", "--quiet", "origin", "main:DEV-2")
	run(t, proto.Dir(), "git", "fetch", "--quiet", "origin")

	if err := SetBranch(ctx, c, "DEV-2"); err != nil {
		t.Fatal(err)
	}
	if c.Branch != "DEV-2" || api.PR != 0 {
		t.Fatalf("branch %q, api PR %d", c.Branch, api.PR)
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

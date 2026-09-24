package change

import (
	"slices"
	"testing"
)

func TestRemoveLegDropsItsDeclaredEdges(t *testing.T) {
	c := &Change{ID: "X", Legs: []Leg{{Repo: "acme/proto"}, {Repo: "acme/grpc"}, {Repo: "acme/api"}}}
	c.AddDeclared(Edge{From: "acme/grpc", To: "acme/api"})
	c.AddDeclared(Edge{From: "acme/proto", To: "acme/api"})

	removed, err := c.RemoveLeg("grpc")
	if err != nil || removed.Repo != "acme/grpc" {
		t.Fatalf("RemoveLeg = %+v, %v", removed, err)
	}
	if len(c.Legs) != 2 || !slices.Equal(c.Declared, []Edge{{From: "acme/proto", To: "acme/api"}}) {
		t.Errorf("after remove: legs %v, declared %v", c.Legs, c.Declared)
	}
	if _, err := c.RemoveLeg("grpc"); err == nil {
		t.Error("removing a missing leg succeeded")
	}
	if !c.RemoveDeclared(Edge{From: "acme/proto", To: "acme/api"}) || len(c.Declared) != 0 {
		t.Errorf("RemoveDeclared left %v", c.Declared)
	}
	if c.RemoveDeclared(Edge{From: "acme/proto", To: "acme/api"}) {
		t.Error("removed an edge twice")
	}
}

func TestUsesWorktrees(t *testing.T) {
	cases := []struct {
		c    Change
		want bool
	}{
		{Change{Worktrees: true}, true},
		{Change{Legs: []Leg{{Worktree: "/w/a"}, {Worktree: "/w/b"}}}, true},
		{Change{Legs: []Leg{{Worktree: "/w/a"}, {Source: "/r/b"}}}, false},
		{Change{}, false},
	}
	for i, tc := range cases {
		if got := tc.c.UsesWorktrees(); got != tc.want {
			t.Errorf("case %d: UsesWorktrees = %v, want %v", i, got, tc.want)
		}
	}
}

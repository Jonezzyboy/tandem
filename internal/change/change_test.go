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

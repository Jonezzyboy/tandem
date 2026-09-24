package graph

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLevels(t *testing.T) {
	names := []string{"monolith", "orchestrator", "connector", "proto"}
	edges := []Edge{
		{From: "proto", To: "orchestrator"},
		{From: "proto", To: "connector"},
		{From: "orchestrator", To: "monolith"},
		{From: "proto", To: "orchestrator"},
		{From: "proto", To: "unknown"},
	}
	got, err := Levels(names, edges)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"proto": 1, "orchestrator": 2, "connector": 2, "monolith": 3}
	for n, l := range want {
		if got[n] != l {
			t.Errorf("level[%s] = %d, want %d", n, got[n], l)
		}
	}
}

func TestLevelsCycle(t *testing.T) {
	_, err := Levels([]string{"a", "b", "c"}, []Edge{{From: "a", To: "b"}, {From: "b", To: "a"}})
	if err == nil || err.Error() != "dependency cycle between a, b" {
		t.Fatalf("err = %v", err)
	}
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

func TestReadManifestAndInfer(t *testing.T) {
	root := t.TempDir()
	proto := filepath.Join(root, "proto")
	write(t, filepath.Join(proto, "go.mod"), "module github.com/acme/proto\n\ngo 1.26\n")
	orch := filepath.Join(root, "orchestrator")
	write(t, filepath.Join(orch, "go.mod"), "module github.com/acme/orchestrator\n\ngo 1.26\n\nrequire github.com/acme/proto v1.30.0\n")
	mono := filepath.Join(root, "mono")
	write(t, filepath.Join(mono, "backend", "composer.json"), `{"name":"acme/backend","require":{"acme/sdk":"^2"}}`)
	write(t, filepath.Join(mono, "frontend", "package.json"), `{"name":"@acme/web","devDependencies":{"@acme/ui":"1.0.0"}}`)
	write(t, filepath.Join(mono, "node_modules", "x", "package.json"), `{"name":"ignored"}`)
	ui := filepath.Join(root, "ui")
	write(t, filepath.Join(ui, "package.json"), `{"name":"@acme/ui"}`)

	var nodes []Node
	for _, d := range []string{proto, orch, mono, ui} {
		m, err := ReadManifest(d)
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, Node{Name: filepath.Base(d), Manifest: m})
	}
	if p := nodes[2].Manifest.Provides; !slices.Contains(p, "acme/backend") || !slices.Contains(p, "@acme/web") || slices.Contains(p, "ignored") {
		t.Errorf("mono provides %v", p)
	}
	got := Infer(nodes)
	want := []Edge{
		{From: "proto", To: "orchestrator", Via: "github.com/acme/proto"},
		{From: "ui", To: "mono", Via: "@acme/ui"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("edges = %v, want %v", got, want)
	}
}

func TestReadManifestMalformed(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "package.json"), `{`)
	if _, err := ReadManifest(dir); err == nil {
		t.Fatal("want error for malformed package.json")
	}
}

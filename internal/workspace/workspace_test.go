package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "acme", "orchestrator"))
	mkRepo(t, filepath.Join(root, "acme", "grpc"))
	mkRepo(t, filepath.Join(root, "globex", "grpc"))
	mkRepo(t, filepath.Join(root, ".tandem", "DEV-1", "orchestrator"))

	r, err := Resolve([]string{root}, "orchestrator")
	if err != nil || r.Name != "acme/orchestrator" {
		t.Fatalf("bare name: %+v, %v", r, err)
	}
	if r, err := Resolve([]string{root}, "globex/grpc"); err != nil || r.Path != filepath.Join(root, "globex", "grpc") {
		t.Fatalf("vendor/repo: %+v, %v", r, err)
	}
	if _, err := Resolve([]string{root}, "grpc"); err == nil || !strings.Contains(err.Error(), "acme/grpc, globex/grpc") {
		t.Fatalf("ambiguous: %v", err)
	}
	if _, err := Resolve([]string{root}, "missing"); err == nil {
		t.Fatal("missing repo resolved")
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, filepath.Join(root, "acme", "api"))
	mkRepo(t, filepath.Join(root, "acme", "web"))
	mkRepo(t, filepath.Join(root, ".tandem", "DEV-1", "api"))
	if err := os.MkdirAll(filepath.Join(root, "acme", "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range List([]string{root}) {
		names = append(names, r.Name)
	}
	if strings.Join(names, ",") != "acme/api,acme/web" {
		t.Errorf("List = %v", names)
	}
}

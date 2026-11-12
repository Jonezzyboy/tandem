package testrun

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFindBuilds(t *testing.T) {
	root := t.TempDir()
	pkg := func(dir, scripts string) {
		write(t, filepath.Join(root, dir, "package.json"), `{"scripts":{`+scripts+`}}`)
	}
	pkg("", `"build":"vite build"`)
	pkg("desktop/frontend", `"build":"vite build"`)
	pkg("tools", `"lint":"eslint ."`)
	pkg("node_modules/dep", `"build":"tsc"`)
	pkg("a/b/c/d", `"build":"tsc"`)
	var dirs []string
	for _, b := range FindBuilds("acme/app", root) {
		dirs = append(dirs, b.Dir)
		if b.Repo != "acme/app" {
			t.Errorf("repo = %q", b.Repo)
		}
	}
	if !slices.Equal(dirs, []string{"", "desktop/frontend"}) {
		t.Errorf("dirs = %q", dirs)
	}
}

func TestBuildStreamsAndStopsOnFailure(t *testing.T) {
	dir := t.TempDir()
	// A stand-in for npm: install succeeds, build fails.
	npm := filepath.Join(dir, "npm")
	script := "#!/bin/sh\necho \"ran $1\"\n[ \"$1\" = run ] && { echo boom >&2; exit 2; }\nexit 0\n"
	if err := os.WriteFile(npm, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var lines []string
	err := Build(context.Background(), dir, npm, func(l string) { lines = append(lines, l) })
	got := strings.Join(lines, "|")
	if err == nil || got != "$ npm install --no-audit --no-fund|ran install|$ npm run build|ran run|boom" {
		t.Errorf("err %v, lines %q", err, got)
	}
}

func TestBuildUsesCIWithLockfile(t *testing.T) {
	dir := t.TempDir()
	npm := filepath.Join(dir, "npm")
	os.WriteFile(npm, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644)
	var lines []string
	if err := Build(context.Background(), dir, npm, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if lines[0] != "$ npm ci --no-audit --no-fund" {
		t.Errorf("first line %q", lines[0])
	}
}

func TestBuildStopKillsChildren(t *testing.T) {
	dir := t.TempDir()
	// Like npm: a child keeps the output open after the parent is killed.
	npm := filepath.Join(dir, "npm")
	os.WriteFile(npm, []byte("#!/bin/sh\nsleep 30 &\nwait\n"), 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	Build(ctx, dir, npm, func(string) {})
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("stop took %v", d)
	}
}

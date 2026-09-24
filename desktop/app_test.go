package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

func TestWithin(t *testing.T) {
	home := t.TempDir()
	a := NewApp(change.Store{Home: home}, nil)
	if _, err := a.within(filepath.Join(home, "ABC-1", "api")); err != nil {
		t.Errorf("path under the tandem home rejected: %v", err)
	}
	repo := t.TempDir()
	c := &change.Change{ID: "ABC-2", Branch: "ABC-2", Legs: []change.Leg{{Repo: "acme/api", Source: repo}}}
	if err := a.store.Save(c); err != nil {
		t.Fatal(err)
	}
	if _, err := a.within(filepath.Join(repo, "cmd")); err != nil {
		t.Errorf("a leg's repo rejected: %v", err)
	}
	for _, p := range []string{filepath.Dir(home), filepath.Join(home, "..", "x"), "/etc"} {
		if _, err := a.within(p); err == nil {
			t.Errorf("%s allowed", p)
		}
	}
}

func TestMergePath(t *testing.T) {
	got := mergePath("/a:/b", "/b:/c::/a:/d")
	if got != "/a:/b:/c:/d" {
		t.Errorf("mergePath = %q", got)
	}
}

func TestSameViewIgnoresCheckedAt(t *testing.T) {
	a := core.ChangeView{ID: "X", CheckedAt: time.Now()}
	b := a
	b.CheckedAt = a.CheckedAt.Add(time.Minute)
	if !sameView(a, b) {
		t.Error("views differing only in CheckedAt reported as changed")
	}
	b.Blocked = 1
	if sameView(a, b) {
		t.Error("different views reported as same")
	}
}

func TestMergeLocalKeepsGitHubState(t *testing.T) {
	remoteAt := time.Now().Add(-time.Minute)
	prev := core.ChangeView{Remote: true, RemoteAt: remoteAt, Blocked: 1, Legs: []core.LegView{{
		Repo: "acme/api", PR: &core.PRView{Number: 7, State: "OPEN"}, Blockers: []string{"awaiting review"},
	}}}
	next := core.ChangeView{Legs: []core.LegView{{Repo: "acme/api", Dirty: 2, OnBranch: true, Blockers: []string{}}}}
	got := core.MergeLocal(prev, next)
	l := got.Legs[0]
	if !got.Remote || !got.RemoteAt.Equal(remoteAt) || l.PR == nil || l.PR.Number != 7 {
		t.Fatalf("GitHub state lost: %+v", got)
	}
	if len(l.Blockers) != 2 || l.Blockers[1] != "uncommitted changes" || got.Blocked != 1 {
		t.Errorf("blockers = %v, blocked = %d", l.Blockers, got.Blocked)
	}
}

func TestPlural(t *testing.T) {
	if plural(1, "leg") != "1 leg" || plural(3, "leg") != "3 legs" {
		t.Error(plural(1, "leg"), plural(3, "leg"))
	}
}

func TestSettingsRoundTripAndNormalize(t *testing.T) {
	t.Setenv("TANDEM_CONFIG_DIR", t.TempDir())
	if got := loadSettings(); got.Theme != "graphite" || got.MergeMethod != "squash" || !got.DraftPRs {
		t.Fatalf("defaults = %+v", got)
	}
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	saved, err := a.SaveSettings(Settings{Theme: "nope", MergeMethod: "yolo", Editors: map[string]string{"other": "cmd:zed"}, Keys: map[string]string{"syncAll": "meta+shift+y"}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Theme != "graphite" || saved.MergeMethod != "squash" || saved.Editors["other"] != "cmd:zed" {
		t.Errorf("normalize = %+v", saved)
	}
	if got := loadSettings(); got.Keys["syncAll"] != "meta+shift+y" || got.Editors["other"] != "cmd:zed" {
		t.Errorf("reloaded = %+v", got)
	}
	path, _ := settingsPath()
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); got.Theme != "graphite" {
		t.Errorf("corrupt file should fall back to defaults, got %+v", got)
	}
}

func TestWindowLook(t *testing.T) {
	bg, appearance := windowLook(Settings{Theme: "paper"})
	if bg.R != 243 || appearance != mac.NSAppearanceNameAqua {
		t.Errorf("paper = %+v %q", bg, appearance)
	}
	if _, appearance := windowLook(Settings{Theme: "midnight"}); appearance != mac.NSAppearanceNameDarkAqua {
		t.Errorf("midnight appearance = %q", appearance)
	}
	if _, appearance := windowLook(Settings{Theme: "system"}); appearance != mac.DefaultAppearance {
		t.Errorf("system appearance = %q", appearance)
	}
}

func TestEditorCommand(t *testing.T) {
	apps := map[string]string{"com.jetbrains.goland": "/Apps/GoLand.app", "com.jetbrains.PhpStorm": "/Apps/PhpStorm.app"}
	editorsOnce = sync.Once{}
	findApp = func(bundle, _ string) string { return apps[bundle] }
	t.Cleanup(func() { editorsOnce = sync.Once{} })

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "fake-editor"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("TANDEM_EDITOR", "")

	repo := func(files ...string) string {
		dir := t.TempDir()
		for _, f := range files {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, f), []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	goRepo, phpRepo, jsRepo, plain := repo("go.mod"), repo("backend/composer.json", "frontend/package.json"), repo("package.json"), repo("README.md")

	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	a.settings = normalize(Settings{Editors: map[string]string{"other": "cmd:fake-editor --wait"}})
	check := func(dir string, want ...string) {
		t.Helper()
		got, err := a.editorCommand(dir)
		if err != nil || strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("editorCommand(%s) = %v, %v; want %v", filepath.Base(dir), got, err, want)
		}
	}
	check(goRepo, "open", "-a", "/Apps/GoLand.app", goRepo)
	check(phpRepo, "open", "-a", "/Apps/PhpStorm.app", phpRepo)
	// WebStorm is not installed, so JS falls back to the "other" command.
	check(jsRepo, filepath.Join(bin, "fake-editor"), "--wait", jsRepo)
	check(plain, filepath.Join(bin, "fake-editor"), "--wait", plain)

	a.settings.Editors["other"] = "webstorm"
	if _, err := a.editorCommand(jsRepo); err == nil || !strings.Contains(err.Error(), "WebStorm isn't installed") {
		t.Errorf("missing app everywhere: %v", err)
	}
}

func TestEditorSettingsDefaultsAndMigration(t *testing.T) {
	d := defaultSettings()
	if d.Editors["go"] != "goland" || d.Editors["php"] != "phpstorm" || d.Editors["js"] != "webstorm" || d.Editors["other"] != "cmd:" {
		t.Errorf("defaults = %v", d.Editors)
	}
	old := normalize(Settings{Editor: "  zed  "})
	if old.Editors["other"] != "cmd:zed" || old.Editor != "" || old.Editors["go"] != "goland" {
		t.Errorf("migration = %+v", old)
	}
	kept := normalize(Settings{Editors: map[string]string{"go": "vscode"}})
	if kept.Editors["go"] != "vscode" || kept.Editors["php"] != "phpstorm" {
		t.Errorf("partial editors = %v", kept.Editors)
	}
}

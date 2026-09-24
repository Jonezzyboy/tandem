package main

import (
	"os"
	"path/filepath"
	"strings"
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
	saved, err := a.SaveSettings(Settings{Theme: "nope", MergeMethod: "yolo", Editor: "  zed  ", Keys: map[string]string{"syncAll": "meta+shift+y"}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Theme != "graphite" || saved.MergeMethod != "squash" || saved.Editor != "zed" {
		t.Errorf("normalize = %+v", saved)
	}
	if got := loadSettings(); got.Keys["syncAll"] != "meta+shift+y" || got.Editor != "zed" {
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
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "fake-editor"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("TANDEM_EDITOR", "")
	a := NewApp(change.Store{Home: t.TempDir()}, nil)
	a.settings.Editor = "fake-editor --wait"
	cmd, err := a.editorCommand()
	if err != nil || cmd[0] != filepath.Join(bin, "fake-editor") || cmd[1] != "--wait" {
		t.Errorf("editorCommand = %v, %v", cmd, err)
	}
	a.settings.Editor = ""
	if _, err := a.editorCommand(); err == nil || !strings.Contains(err.Error(), `"code" not found`) {
		t.Errorf("missing default editor: %v", err)
	}
}

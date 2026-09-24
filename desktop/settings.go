package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// version is set by scripts/build-app.sh with -ldflags "-X main.version=...".
var version = "dev"

type Settings struct {
	Theme string `json:"theme"`
	// Keys maps an action id to a shortcut like "meta+shift+s". Actions left
	// out use the frontend's defaults, so new actions need no migration.
	Keys        map[string]string `json:"keys"`
	Editor      string            `json:"editor"`
	MergeMethod string            `json:"mergeMethod"`
	DraftPRs    bool              `json:"draftPRs"`
}

// theme is a theme's window background and whether macOS should draw it dark.
type theme struct {
	bg   options.RGBA
	dark bool
}

// Keep in sync with the [data-theme] blocks in frontend/src/themes.css.
var themes = map[string]theme{
	"graphite": {options.RGBA{R: 17, G: 18, B: 22, A: 255}, true},
	"midnight": {options.RGBA{R: 14, G: 19, B: 32, A: 255}, true},
	"forest":   {options.RGBA{R: 16, G: 21, B: 18, A: 255}, true},
	"contrast": {options.RGBA{R: 0, G: 0, B: 0, A: 255}, true},
	"paper":    {options.RGBA{R: 243, G: 241, B: 234, A: 255}, false},
}

func defaultSettings() Settings {
	return Settings{Theme: "graphite", Keys: map[string]string{}, MergeMethod: "squash", DraftPRs: true}
}

// settingsPath is ~/Library/Application Support/com.alanjones.tandem/settings.json,
// or $TANDEM_CONFIG_DIR/settings.json when set. The folder is named for the
// bundle ID: "Tandem" is already used by an unrelated app.
func settingsPath() (string, error) {
	if dir := os.Getenv("TANDEM_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "com.alanjones.tandem", "settings.json"), nil
}

// loadSettings falls back to defaults for a missing or unreadable file, so a
// bad edit never stops the app from starting.
func loadSettings() Settings {
	s := defaultSettings()
	path, err := settingsPath()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if json.Unmarshal(data, &s) != nil {
		return defaultSettings()
	}
	return normalize(s)
}

func normalize(s Settings) Settings {
	if _, ok := themes[s.Theme]; !ok && s.Theme != "system" {
		s.Theme = "graphite"
	}
	switch s.MergeMethod {
	case "squash", "merge", "rebase":
	default:
		s.MergeMethod = "squash"
	}
	if s.Keys == nil {
		s.Keys = map[string]string{}
	}
	s.Editor = strings.TrimSpace(s.Editor)
	return s
}

func saveSettings(s Settings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// systemDark reports macOS dark mode. AppleInterfaceStyle is only set in dark
// mode, so a failed read means light.
func systemDark() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle").Output()
	return err == nil && strings.TrimSpace(string(out)) == "Dark"
}

func resolveTheme(name string) theme {
	if name == "system" {
		if systemDark() {
			return themes["graphite"]
		}
		return themes["paper"]
	}
	if t, ok := themes[name]; ok {
		return t
	}
	return themes["graphite"]
}

// windowLook is the startup background and appearance, so the first frame is
// already the saved theme rather than a flash of the default.
func windowLook(s Settings) (*options.RGBA, mac.AppearanceType) {
	t := resolveTheme(s.Theme)
	bg := t.bg
	if s.Theme == "system" {
		return &bg, mac.DefaultAppearance
	}
	if t.dark {
		return &bg, mac.NSAppearanceNameDarkAqua
	}
	return &bg, mac.NSAppearanceNameAqua
}

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *App) SaveSettings(s Settings) (Settings, error) {
	s = normalize(s)
	if err := saveSettings(s); err != nil {
		return a.Settings(), err
	}
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	if a.ctx != nil {
		bg := resolveTheme(s.Theme).bg
		runtime.WindowSetBackgroundColour(a.ctx, bg.R, bg.G, bg.B, bg.A)
	}
	return s, nil
}

func (a *App) SettingsPath() string {
	p, _ := settingsPath()
	return p
}

// SystemDark lets the frontend resolve the "system" theme; the webview's own
// prefers-color-scheme follows the window's forced appearance, not macOS.
func (a *App) SystemDark() bool {
	return systemDark()
}

func (a *App) Version() string {
	return version
}

// TandemHome is where changes and their worktrees live.
func (a *App) TandemHome() string {
	return a.store.Home
}

type Account struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
	Error     string `json:"error"`
}

// Account is the GitHub user gh is signed in as, looked up once per launch.
func (a *App) Account() Account {
	a.mu.Lock()
	cached := a.account
	a.mu.Unlock()
	if cached != nil {
		return *cached
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	u, err := gh.CurrentUser(ctx)
	acc := Account{Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL}
	if err != nil {
		acc.Error = "gh is not signed in: run gh auth login"
		var ee *exec.Error
		if errors.As(err, &ee) || errors.Is(err, fs.ErrNotExist) {
			acc.Error = "gh is not installed"
		}
		return acc
	}
	a.mu.Lock()
	a.account = &acc
	a.mu.Unlock()
	return acc
}

// editorCommand is the saved editor, else $TANDEM_EDITOR, else code; it may
// carry arguments ("open -a Cursor").
func (a *App) editorCommand() ([]string, error) {
	cmd := a.Settings().Editor
	if cmd == "" {
		cmd = os.Getenv("TANDEM_EDITOR")
	}
	if cmd == "" {
		cmd = "code"
	}
	parts := strings.Fields(cmd)
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, fmt.Errorf("editor %q not found: set it in Settings → General", parts[0])
	}
	return append([]string{bin}, parts[1:]...), nil
}

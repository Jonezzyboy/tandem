package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultWidth, defaultHeight = 1320, 860
	minWidth, minHeight         = 980, 620
)

// windowState is the window as it was last closed, kept apart from settings
// because the frontend saves those whole from its own copy. X and Y are
// relative to the screen's visible area, as Wails reports and sets them.
type windowState struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

func windowPath() (string, error) {
	p, err := settingsPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "window.json"), nil
}

// loadWindow returns the saved state, or the default size and ok false when
// there is none to restore.
func loadWindow() (windowState, bool) {
	s := windowState{Width: defaultWidth, Height: defaultHeight}
	path, err := windowPath()
	if err != nil {
		return s, false
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &s) != nil {
		return windowState{Width: defaultWidth, Height: defaultHeight}, false
	}
	s.Width, s.Height = max(s.Width, minWidth), max(s.Height, minHeight)
	return s, true
}

func saveWindow(s windowState) error {
	path, err := windowPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// restoreWindow shows the window, which starts hidden, and puts it back where
// it was closed, kept on the current screen in case the one it was on has gone.
// Placing comes after showing because a hidden window has no screen to measure
// against. It runs once, as DOM ready also fires on reloads.
func (a *App) restoreWindow(ctx context.Context) {
	a.restoreOnce.Do(func() {
		runtime.WindowShow(ctx)
		a.placeWindow(ctx)
	})
}

func (a *App) placeWindow(ctx context.Context) {
	s, ok := loadWindow()
	if !ok {
		return
	}
	if screens, err := runtime.ScreenGetAll(ctx); err == nil {
		for _, sc := range screens {
			if !sc.IsCurrent {
				continue
			}
			w, h := min(s.Width, sc.Size.Width), min(s.Height, sc.Size.Height)
			if w != s.Width || h != s.Height {
				runtime.WindowSetSize(ctx, w, h)
			}
			s.X = min(max(s.X, 0), max(sc.Size.Width-w, 0))
			s.Y = min(max(s.Y, 0), max(sc.Size.Height-h, 0))
		}
	}
	runtime.WindowSetPosition(ctx, s.X, s.Y)
	if s.Maximised {
		runtime.WindowMaximise(ctx)
	}
}

// rememberWindow saves the window's size and place as it closes. A maximised
// or full-screen window keeps the normal size saved before, to return to.
func (a *App) rememberWindow(ctx context.Context) bool {
	prev, _ := loadWindow()
	s := prev
	s.Maximised = runtime.WindowIsMaximised(ctx)
	if !s.Maximised && !runtime.WindowIsFullscreen(ctx) && !runtime.WindowIsMinimised(ctx) {
		s.Width, s.Height = runtime.WindowGetSize(ctx)
		s.X, s.Y = runtime.WindowGetPosition(ctx)
	}
	saveWindow(s)
	return false
}

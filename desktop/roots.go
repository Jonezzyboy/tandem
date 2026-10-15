package main

import (
	"github.com/jonezzyboy/tandem/internal/workspace"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// codeRoots are the folders searched for clones: the ones chosen in Settings,
// else $TANDEM_ROOT or ~/code.
func (a *App) codeRoots() []string {
	if r := a.Settings().Roots; len(r) > 0 {
		return r
	}
	return a.roots
}

type RootInfo struct {
	Path  string `json:"path"`
	Repos int    `json:"repos"`
}

// CodeRoots lists the folders searched for clones, each with how many repos it holds.
func (a *App) CodeRoots() []RootInfo {
	roots := a.codeRoots()
	out := make([]RootInfo, len(roots))
	for i, r := range roots {
		out[i] = RootInfo{Path: r, Repos: len(workspace.List([]string{r}))}
	}
	return out
}

// DefaultRoots are the folders used when Settings chooses none.
func (a *App) DefaultRoots() []string {
	return a.roots
}

// ChooseFolder asks for a folder; "" means the dialog was cancelled.
func (a *App) ChooseFolder(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

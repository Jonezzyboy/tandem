package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const pathMarker = "__TANDEM_PATH__"

// adoptShellPath appends the entries of an interactive login shell's PATH to
// the current one. A Finder launch starts with launchd's bare PATH, where git,
// gh, go and version-managed node are missing; nvm/fnm/volta only initialise
// from the interactive rc files, hence -i. Markers fence off anything rc files
// print.
func adoptShellPath() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-ilc", `printf '`+pathMarker+`%s`+pathMarker+`' "$PATH"`).Output()
	if err != nil {
		return
	}
	s := string(out)
	i := strings.Index(s, pathMarker)
	j := strings.LastIndex(s, pathMarker)
	if i < 0 || j <= i {
		return
	}
	os.Setenv("PATH", mergePath(os.Getenv("PATH"), s[i+len(pathMarker):j]))
}

func mergePath(current, extra string) string {
	parts := filepath.SplitList(current)
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		seen[p] = true
	}
	for _, p := range filepath.SplitList(extra) {
		if p != "" && !seen[p] {
			seen[p] = true
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, string(filepath.ListSeparator))
}

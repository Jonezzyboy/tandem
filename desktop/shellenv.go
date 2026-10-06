package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const envMarker = "__TANDEM_ENV__"

// adoptShellEnv takes on an interactive login shell's environment. A Finder
// launch starts with launchd's bare one, missing PATH entries for git, gh, go
// and version-managed node, and settings such as GOPRIVATE that go get needs
// for private modules. nvm/fnm/volta only initialise from the interactive rc
// files, hence -i. Markers fence off anything rc files print.
func adoptShellEnv() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-ilc", `printf '`+envMarker+`'; env -0; printf '`+envMarker+`'`).Output()
	if err != nil {
		return
	}
	s := string(out)
	i := strings.Index(s, envMarker)
	j := strings.LastIndex(s, envMarker)
	if i < 0 || j <= i {
		return
	}
	for k, v := range shellVars(s[i+len(envMarker) : j]) {
		if k == "PATH" {
			os.Setenv("PATH", mergePath(os.Getenv("PATH"), v))
		} else if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

// shellVars parses env -0 output, leaving out what describes the probe shell
// itself rather than the user's settings.
func shellVars(out string) map[string]string {
	vars := map[string]string{}
	for _, kv := range strings.Split(out, "\x00") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		switch k {
		case "PWD", "OLDPWD", "SHLVL", "_":
			continue
		}
		vars[k] = v
	}
	return vars
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

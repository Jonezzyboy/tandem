package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SetupItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	// Fix is the action the checklist offers: "terminal", "folder",
	// "notifications" or "settings".
	Fix string `json:"fix,omitempty"`
}

// Setup checks what Tandem needs on this machine, for the first-run checklist.
func (a *App) Setup() []SetupItem {
	a.mu.Lock()
	a.account = nil
	a.mu.Unlock()
	return []SetupItem{a.setupGit(), a.setupGH(), a.setupRoots(), a.setupNotifications(), a.setupJira()}
}

func (a *App) setupGit() SetupItem {
	it := SetupItem{ID: "git", Title: "git", Required: true}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "--version").Output()
	if err != nil {
		it.Detail, it.Fix = "Not found. Install the Xcode command line tools.", "terminal"
		return it
	}
	it.OK, it.Detail = true, strings.TrimSpace(string(out))
	return it
}

func (a *App) setupGH() SetupItem {
	it := SetupItem{ID: "gh", Title: "GitHub CLI", Required: true, Fix: "terminal"}
	acc := a.Account()
	switch {
	case acc.Error == "gh is not installed":
		it.Detail = "Not installed. Tandem opens PRs and reads CI through gh."
	case acc.Error != "":
		it.Detail = "Not signed in. Sign in once in Terminal."
	default:
		it.OK, it.Fix, it.Detail = true, "", "Signed in as @"+acc.Login
	}
	return it
}

func (a *App) setupRoots() SetupItem {
	it := SetupItem{ID: "roots", Title: "Your repositories", Required: true, Fix: "folder"}
	n := 0
	var paths []string
	for _, r := range a.CodeRoots() {
		n += r.Repos
		paths = append(paths, tilde(r.Path))
	}
	where := strings.Join(paths, ", ")
	if n == 0 {
		it.Detail = fmt.Sprintf("No clones found in %s. Choose the folder that holds them as <owner>/<repo>.", where)
		return it
	}
	it.OK, it.Detail = true, fmt.Sprintf("%d repos in %s", n, where)
	return it
}

func (a *App) setupNotifications() SetupItem {
	it := SetupItem{ID: "notifications", Title: "Notifications", Fix: "notifications"}
	st := a.NotificationStatus()
	switch {
	case !st.Available:
		it.Fix, it.Detail = "", "Not available in this build."
	case st.Allowed:
		it.OK, it.Fix, it.Detail = true, "", "On: CI results, reviews, merges and review requests."
	default:
		it.Detail = "Hear about failing CI, approvals, merges and review requests while you work elsewhere."
	}
	return it
}

func (a *App) setupJira() SetupItem {
	it := SetupItem{ID: "jira", Title: "Jira", Fix: "settings"}
	s := a.Settings()
	if s.Jira.Site != "" && s.Jira.Email != "" && jiraToken(s.Jira.Email) != "" {
		it.OK, it.Fix, it.Detail = true, "", "Signed in to "+strings.TrimPrefix(s.Jira.Site, "https://")
		return it
	}
	it.Detail = "Fills a change's title and description from its ticket."
	return it
}

func tilde(path string) string {
	home, _ := os.UserHomeDir()
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}

// setupCommands are the only commands the checklist can run in Terminal.
var setupCommands = map[string]string{
	"git": "xcode-select --install",
	"gh":  "command -v gh >/dev/null || brew install gh; gh auth login",
}

// RunSetupFix opens Terminal on the fix for a checklist item. A .command file
// opens in Terminal without asking for permission to automate it.
func (a *App) RunSetupFix(id string) error {
	cmd, ok := setupCommands[id]
	if !ok {
		return fmt.Errorf("no fix for %s", id)
	}
	path := filepath.Join(os.TempDir(), "tandem-setup-"+id+".command")
	script := fmt.Sprintf("#!/bin/zsh -il\nclear\necho '$ %s'\n%s\necho\necho 'Done. Switch back to Tandem and it will recheck.'\n", cmd, cmd)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return err
	}
	return exec.Command("open", path).Run()
}

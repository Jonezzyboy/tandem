// Package workspace finds repo clones under the code roots and creates the
// per-change git worktrees.
package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonezzyboy/tandem/internal/gitx"
)

// Roots are the directories holding <vendor>/<repo> clones: $TANDEM_ROOT
// (path-list separated) or ~/code.
func Roots() []string {
	if v := os.Getenv("TANDEM_ROOT"); v != "" {
		return filepath.SplitList(v)
	}
	home, _ := os.UserHomeDir()
	return []string{filepath.Join(home, "code")}
}

// Home is where changes and their worktrees live: $TANDEM_HOME or ~/code/.tandem.
func Home() string {
	if v := os.Getenv("TANDEM_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "code", ".tandem")
}

type Repo struct {
	Name string // vendor/repo
	Path string
}

func isRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// Resolve accepts "vendor/repo" or a bare repo name.
func Resolve(roots []string, arg string) (Repo, error) {
	if strings.Contains(arg, "/") {
		for _, root := range roots {
			if p := filepath.Join(root, arg); isRepo(p) {
				return Repo{Name: arg, Path: p}, nil
			}
		}
		return Repo{}, fmt.Errorf("no clone of %s under %s", arg, strings.Join(roots, ", "))
	}
	var found []Repo
	for _, root := range roots {
		vendors, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, v := range vendors {
			// Dot dirs include the tandem home, whose worktrees would otherwise match.
			if !v.IsDir() || strings.HasPrefix(v.Name(), ".") {
				continue
			}
			if p := filepath.Join(root, v.Name(), arg); isRepo(p) {
				found = append(found, Repo{Name: v.Name() + "/" + arg, Path: p})
			}
		}
	}
	switch len(found) {
	case 0:
		return Repo{}, fmt.Errorf("no repo named %s under %s", arg, strings.Join(roots, ", "))
	case 1:
		return found[0], nil
	}
	names := make([]string, len(found))
	for i, r := range found {
		names[i] = r.Name
	}
	return Repo{}, fmt.Errorf("%s is ambiguous: %s", arg, strings.Join(names, ", "))
}

// AddWorktree checks out branch at dst, reusing a local or remote branch of
// that name before cutting a new one from base. It reports false when dst is
// already a worktree.
func AddWorktree(ctx context.Context, src, dst, branch string, base gitx.Base) (bool, error) {
	if isRepo(dst) {
		return false, nil
	}
	var args []string
	switch {
	case gitx.RefExists(ctx, src, "refs/heads/"+branch):
		args = []string{"worktree", "add", "--quiet", dst, branch}
	case gitx.RefExists(ctx, src, "refs/remotes/origin/"+branch):
		args = []string{"worktree", "add", "--quiet", "--track", "-b", branch, dst, "origin/" + branch}
	default:
		args = []string{"worktree", "add", "--quiet", "--no-track", "-b", branch, dst, base.Ref}
	}
	if _, err := gitx.Run(ctx, src, args...); err != nil {
		return false, err
	}
	return true, nil
}

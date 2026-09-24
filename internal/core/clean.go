package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gitx"
)

type WorktreeRef struct {
	Path   string
	Source string
}

type BranchRef struct {
	Repo   string
	Source string
	Branch string
}

// CleanCandidate is one change and exactly what cleaning it would remove.
// Only Ready candidates may be cleaned; Reason says why one is not.
type CleanCandidate struct {
	Change    *change.Change
	Ready     bool
	Reason    string
	Worktrees []WorktreeRef
	Branches  []BranchRef
	// Kept are local branches left alone, e.g. for a PR closed unmerged.
	Kept  []BranchRef
	Files []string
	Dir   string
}

// CleanCandidates checks every change. A change is ready when each leg's PR
// merged or closed (or it never had commits), and no worktree holds
// uncommitted or unpushed work.
func CleanCandidates(ctx context.Context, store change.Store) ([]CleanCandidate, error) {
	all, err := store.List()
	if err != nil {
		return nil, err
	}
	out := make([]CleanCandidate, 0, len(all))
	for _, c := range all {
		out = append(out, candidate(ctx, store, c))
	}
	return out, nil
}

func candidate(ctx context.Context, store change.Store, c *change.Change) CleanCandidate {
	cc := CleanCandidate{Change: c, Dir: store.Dir(c.ID), Ready: true}
	var why []string
	for _, s := range Snapshot(ctx, c, true) {
		l := s.Leg
		ref := BranchRef{Repo: l.Repo, Source: l.Source, Branch: c.Branch}
		switch {
		case s.StatusErr != nil:
			why = append(why, l.Name()+": "+FirstLine(s.StatusErr.Error()))
		case s.PRErr != nil:
			why = append(why, l.Name()+": "+FirstLine(s.PRErr.Error()))
		case s.Status.Dirty > 0:
			why = append(why, fmt.Sprintf("%s: %d uncommitted files", l.Name(), s.Status.Dirty))
		case unpushed(ctx, c, l) > 0:
			why = append(why, l.Name()+": commits not pushed")
		case s.PR == nil && s.Status.Ahead == 0:
			cc.Branches = append(cc.Branches, ref)
		case s.PR == nil:
			why = append(why, fmt.Sprintf("%s: %d commits and no PR", l.Name(), s.Status.Ahead))
		case s.PR.State == "MERGED":
			cc.Branches = append(cc.Branches, ref)
		case s.PR.State == "CLOSED":
			cc.Kept = append(cc.Kept, ref)
		default:
			why = append(why, l.Name()+": PR #"+strconv.Itoa(s.PR.Number)+" still open")
		}
		cc.Worktrees = append(cc.Worktrees, WorktreeRef{Path: l.Worktree, Source: l.Source})
	}
	if len(why) > 0 {
		cc.Ready, cc.Reason = false, strings.Join(why, "; ")
	}
	legDirs := map[string]bool{}
	for _, w := range cc.Worktrees {
		legDirs[filepath.Clean(w.Path)] = true
	}
	entries, _ := os.ReadDir(cc.Dir)
	for _, e := range entries {
		p := filepath.Join(cc.Dir, e.Name())
		if !e.IsDir() && !legDirs[p] {
			cc.Files = append(cc.Files, p)
		}
	}
	return cc
}

func unpushed(ctx context.Context, c *change.Change, l *change.Leg) int {
	if !gitx.RefExists(ctx, l.Worktree, "refs/remotes/origin/"+c.Branch) {
		return 0
	}
	out, err := gitx.Run(ctx, l.Worktree, "rev-list", "--count", "refs/remotes/origin/"+c.Branch+"..HEAD")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(out)
	return n
}

// Clean removes exactly what cc lists: each worktree (git refuses one with
// changes, and that refusal stands), each listed local branch, the change's
// files, and its directory if that leaves it empty.
func Clean(ctx context.Context, cc CleanCandidate) error {
	if !cc.Ready {
		return fmt.Errorf("%s is not ready to clean: %s", cc.Change.ID, cc.Reason)
	}
	var errs []error
	for _, w := range cc.Worktrees {
		if _, err := os.Stat(w.Path); errors.Is(err, os.ErrNotExist) {
			gitx.Run(ctx, w.Source, "worktree", "prune")
			continue
		}
		if _, err := gitx.Run(ctx, w.Source, "worktree", "remove", w.Path); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	for _, b := range cc.Branches {
		if gitx.RefExists(ctx, b.Source, "refs/heads/"+b.Branch) {
			if _, err := gitx.Run(ctx, b.Source, "branch", "-D", b.Branch); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, f := range cc.Files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := os.Remove(cc.Dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("left %s in place: %w", cc.Dir, err))
	}
	return errors.Join(errs...)
}

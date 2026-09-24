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

// SwitchRef is a repo that has the change's branch checked out and must move
// to Base before the branch can be deleted.
type SwitchRef struct {
	Repo   string
	Source string
	Base   string
}

// CleanCandidate is one change and exactly what cleaning it would do.
// Only Ready candidates may be cleaned; Reason says why one is not.
type CleanCandidate struct {
	Change   *change.Change
	Ready    bool
	Reason   string
	Switches []SwitchRef
	Branches []BranchRef
	// Kept are local branches left alone, e.g. for a PR closed unmerged.
	Kept []BranchRef
	// Worktrees belong to legs made before changes used branches.
	Worktrees []WorktreeRef
	Files     []string
	Dir       string
}

// CleanCandidates checks every change. A change is ready when each leg's PR
// merged or closed (or it never had commits), and no repo holds uncommitted
// or unpushed work for it.
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
		deletable := false
		switch {
		case s.StatusErr != nil:
			why = append(why, l.Name()+": "+FirstLine(s.StatusErr.Error()))
		case s.PRErr != nil:
			why = append(why, l.Name()+": "+FirstLine(s.PRErr.Error()))
		case s.OnBranch() && s.Status.Dirty > 0:
			why = append(why, fmt.Sprintf("%s: %d uncommitted files on %s", l.Name(), s.Status.Dirty, c.Branch))
		case unpushed(ctx, c, l) > 0:
			why = append(why, l.Name()+": commits not pushed")
		case s.PR == nil && s.Status.Ahead == 0:
			deletable = true
		case s.PR == nil:
			why = append(why, fmt.Sprintf("%s: %d commits and no PR", l.Name(), s.Status.Ahead))
		case s.PR.State == "MERGED":
			deletable = true
		case s.PR.State == "CLOSED":
			cc.Kept = append(cc.Kept, ref)
		default:
			why = append(why, l.Name()+": PR #"+strconv.Itoa(s.PR.Number)+" still open")
		}
		if l.Worktree != "" {
			cc.Worktrees = append(cc.Worktrees, WorktreeRef{Path: l.Worktree, Source: l.Source})
		} else if deletable && s.OnBranch() {
			cc.Switches = append(cc.Switches, SwitchRef{Repo: l.Repo, Source: l.Source, Base: l.Base})
		}
		if deletable {
			cc.Branches = append(cc.Branches, ref)
		}
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
	dir := l.Dir()
	if !gitx.RefExists(ctx, dir, "refs/remotes/origin/"+c.Branch) || !gitx.RefExists(ctx, dir, "refs/heads/"+c.Branch) {
		return 0
	}
	out, err := gitx.Run(ctx, dir, "rev-list", "--count", "refs/remotes/origin/"+c.Branch+"..refs/heads/"+c.Branch)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(out)
	return n
}

// Clean does exactly what cc lists: moves repos off the change's branch onto
// their base (git refuses if that would lose work, and the refusal stands),
// removes any legacy worktrees, deletes the listed local branches and the
// change's files, and its directory if that leaves it empty.
func Clean(ctx context.Context, cc CleanCandidate) error {
	if !cc.Ready {
		return fmt.Errorf("%s is not ready to clean: %s", cc.Change.ID, cc.Reason)
	}
	var errs []error
	for _, s := range cc.Switches {
		if err := gitx.Switch(ctx, s.Source, s.Base); err != nil {
			errs = append(errs, fmt.Errorf("%s: switching to %s: %w", s.Repo, s.Base, err))
		}
	}
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

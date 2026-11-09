package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

// BranchRepo is a repo that already has a change's branch.
type BranchRepo struct {
	Repo workspace.Repo
	// Local and Remote say where the branch is: refs/heads or refs/remotes/origin.
	Local, Remote bool
	// Ahead counts the branch's commits not on the repo's base.
	Ahead int
	// Current is set when the clone has the branch checked out.
	Current bool
}

// scanLimit bounds the git processes FindBranch runs at once.
const scanLimit = 16

// FindBranch reports which repos have branch locally or on origin, by name.
// It reads refs as they are, without fetching, so it stays quick across every
// clone under the roots.
func FindBranch(ctx context.Context, repos []workspace.Repo, branch string) []BranchRepo {
	found := make([]*BranchRepo, len(repos))
	sem := make(chan struct{}, scanLimit)
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			found[i] = findBranch(ctx, r, branch)
		})
	}
	wg.Wait()
	var out []BranchRepo
	for _, b := range found {
		if b != nil {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Repo.Name < out[j].Repo.Name })
	return out
}

func findBranch(ctx context.Context, r workspace.Repo, branch string) *BranchRepo {
	if !gitx.MayHaveRef(r.Path, "refs/heads/"+branch) && !gitx.MayHaveRef(r.Path, "refs/remotes/origin/"+branch) {
		return nil
	}
	b := BranchRepo{
		Repo:   r,
		Local:  gitx.RefExists(ctx, r.Path, "refs/heads/"+branch),
		Remote: gitx.RefExists(ctx, r.Path, "refs/remotes/origin/"+branch),
	}
	if !b.Local && !b.Remote {
		return nil
	}
	ref := "refs/heads/" + branch
	if !b.Local {
		ref = "refs/remotes/origin/" + branch
	}
	if base, err := gitx.DefaultBase(ctx, r.Path); err == nil {
		b.Ahead = gitx.CountAhead(ctx, r.Path, base.Ref, ref)
	}
	b.Current = gitx.CurrentBranch(ctx, r.Path) == branch
	return &b
}

// SetBranch points change c at branch. A leg that already has branch, locally
// or on origin, uses it as it is; any other leg's current branch is renamed to
// it. Worktree legs are moved onto it. Every leg is checked first, and steps
// already taken are undone if one fails; c is updated, not saved.
func SetBranch(ctx context.Context, c *change.Change, branch string) error {
	if branch == c.Branch {
		return nil
	}
	if _, err := gitx.Run(ctx, "", "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("invalid branch name %q", branch)
	}
	type step struct {
		l             *change.Leg
		local, remote bool
	}
	steps := make([]step, len(c.Legs))
	for i := range c.Legs {
		l := &c.Legs[i]
		s := step{l: l,
			local:  gitx.RefExists(ctx, l.Dir(), "refs/heads/"+branch),
			remote: gitx.RefExists(ctx, l.Dir(), "refs/remotes/origin/"+branch)}
		if !s.local && !s.remote && !gitx.RefExists(ctx, l.Dir(), "refs/heads/"+c.Branch) {
			return fmt.Errorf("%s has neither %s nor %s", l.Name(), branch, c.Branch)
		}
		if l.Worktree != "" && (s.local || s.remote) {
			if out, err := gitx.Run(ctx, l.Dir(), "status", "--porcelain", "--untracked-files=no"); err != nil {
				return err
			} else if out != "" {
				return fmt.Errorf("%s has uncommitted work: commit or stash it first", l.Name())
			}
		}
		steps[i] = s
	}
	var undo []func()
	rollback := func(err error) error {
		for _, u := range slices.Backward(undo) {
			u()
		}
		return err
	}
	for _, s := range steps {
		dir := s.l.Dir()
		if !s.local && !s.remote {
			if _, err := gitx.Run(ctx, dir, "branch", "-m", c.Branch, branch); err != nil {
				return rollback(fmt.Errorf("%s: %w", s.l.Name(), err))
			}
			undo = append(undo, func() { gitx.Run(context.WithoutCancel(ctx), dir, "branch", "-m", branch, c.Branch) })
			continue
		}
		if !s.local {
			if _, err := gitx.Run(ctx, dir, "branch", "--quiet", "--track", branch, "origin/"+branch); err != nil {
				return rollback(fmt.Errorf("%s: %w", s.l.Name(), err))
			}
			undo = append(undo, func() { gitx.Run(context.WithoutCancel(ctx), dir, "branch", "-D", branch) })
		}
		if s.l.Worktree != "" {
			if err := gitx.Switch(ctx, dir, branch); err != nil {
				return rollback(fmt.Errorf("%s: %w", s.l.Name(), err))
			}
			undo = append(undo, func() { gitx.Switch(context.WithoutCancel(ctx), dir, c.Branch) })
		}
	}
	c.Branch = branch
	for i := range c.Legs {
		l := &c.Legs[i]
		if l.PR == 0 {
			continue
		}
		// A merged PR stays so the leg isn't published again; any other is on the
		// old branch, and the new branch's PR is found by name.
		if pr, err := gh.View(ctx, l.Dir(), strconv.Itoa(l.PR)); err == nil && pr != nil && pr.State == "MERGED" {
			continue
		}
		l.PR, l.PRURL = 0, ""
	}
	return nil
}

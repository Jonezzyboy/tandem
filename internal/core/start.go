package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

type AddResult struct {
	Repo workspace.Repo
	// Existing is set when the repo was already a leg; nothing was done.
	Existing bool
	Leg      change.Leg
	// Created is false when the branch already existed; Switched is whether
	// the repo now has it checked out.
	Created  bool
	Switched bool
	Warning  string
	Err      error
}

type StartOptions struct {
	// Worktrees applies when the change is created: each leg becomes a worktree
	// under the change's directory, leaving the clones' checkouts alone.
	Worktrees bool
}

// Start loads or creates change id and, per repo concurrently, creates the
// change's branch. By default it is checked out in the repo's clone, and a
// repo with uncommitted work keeps its current branch, with a warning; a
// worktree change instead gets a worktree per repo. It saves the change when
// any leg exists. Repos sharing a bare name are rejected before anything changes.
func Start(ctx context.Context, store change.Store, id, title string, repos []workspace.Repo, o StartOptions) (*change.Change, []AddResult, error) {
	if err := change.ValidateID(id); err != nil {
		return nil, nil, err
	}
	c, err := store.Load(id)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		c = &change.Change{ID: id, Branch: id, Created: time.Now().UTC(), Worktrees: o.Worktrees}
	case err != nil:
		return nil, nil, err
	}
	if title != "" {
		c.Title = title
	}

	names := map[string]string{}
	for _, l := range c.Legs {
		names[l.Name()] = l.Repo
	}
	results := make([]AddResult, len(repos))
	for i, r := range repos {
		results[i].Repo = r
		short := filepath.Base(r.Name)
		if prev, ok := names[short]; ok {
			if prev == r.Name {
				results[i].Existing = true
				continue
			}
			return nil, nil, fmt.Errorf("%s and %s share the name %s; one change can hold only one of them", prev, r.Name, short)
		}
		names[short] = r.Name
	}

	var wg sync.WaitGroup
	for i := range results {
		if results[i].Existing {
			continue
		}
		wg.Go(func() {
			if c.Worktrees {
				addWorktreeLeg(ctx, store.Dir(c.ID), c.Branch, &results[i])
			} else {
				addLeg(ctx, c.Branch, &results[i])
			}
		})
	}
	wg.Wait()
	for _, r := range results {
		if !r.Existing && r.Err == nil {
			c.Legs = append(c.Legs, r.Leg)
		}
	}
	if len(c.Legs) > 0 {
		if err := store.Save(c); err != nil {
			return c, results, err
		}
	}
	return c, results, nil
}

func addLeg(ctx context.Context, branch string, res *AddResult) {
	r := res.Repo
	if gitx.HasRemote(ctx, r.Path, "origin") {
		if err := gitx.Fetch(ctx, r.Path); err != nil {
			res.Warning = "fetch failed, used local refs"
		}
	}
	base, err := gitx.DefaultBase(ctx, r.Path)
	if err != nil {
		res.Err = err
		return
	}
	if res.Created, res.Err = workspace.CreateBranch(ctx, r.Path, branch, base); res.Err != nil {
		return
	}
	res.Leg = change.Leg{Repo: r.Name, Source: r.Path, Base: base.Branch, BaseRef: base.Ref}
	if err := gitx.Switch(ctx, r.Path, branch); err != nil {
		res.Warning = join(res.Warning, "stayed on "+orDetached(gitx.CurrentBranch(ctx, r.Path))+": "+FirstLine(err.Error()))
		return
	}
	res.Switched = true
}

func addWorktreeLeg(ctx context.Context, dir, branch string, res *AddResult) {
	r := res.Repo
	if gitx.HasRemote(ctx, r.Path, "origin") {
		if err := gitx.Fetch(ctx, r.Path); err != nil {
			res.Warning = "fetch failed, used local refs"
		}
	}
	base, err := gitx.DefaultBase(ctx, r.Path)
	if err != nil {
		res.Err = err
		return
	}
	dst := filepath.Join(dir, filepath.Base(r.Name))
	if res.Created, res.Err = workspace.AddWorktree(ctx, r.Path, dst, branch, base); res.Err != nil {
		return
	}
	res.Leg = change.Leg{Repo: r.Name, Source: r.Path, Worktree: dst, Base: base.Branch, BaseRef: base.Ref}
	res.Switched = true
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func orDetached(branch string) string {
	if branch == "" {
		return "a detached HEAD"
	}
	return branch
}

type SwitchResult struct {
	Leg *change.Leg
	// To is the branch the repo was asked onto; Err says why it stayed put.
	To  string
	Err error
	// Skipped is set for worktree legs, which always have the change's branch.
	Skipped bool
}

// Switch checks out the change's branch in every leg, or with toBase each
// leg's base branch. Legs with uncommitted work are left as they are, and
// worktree legs are skipped.
func Switch(ctx context.Context, c *change.Change, toBase bool) []SwitchResult {
	out := make([]SwitchResult, len(c.Legs))
	var wg sync.WaitGroup
	for i := range c.Legs {
		l := &c.Legs[i]
		to := c.Branch
		if toBase {
			to = l.Base
		}
		out[i] = SwitchResult{Leg: l, To: to}
		if l.Worktree != "" {
			out[i].To, out[i].Skipped = c.Branch, true
			continue
		}
		wg.Go(func() { out[i].Err = gitx.Switch(ctx, l.Dir(), to) })
	}
	wg.Wait()
	return out
}

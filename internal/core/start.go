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
	Warning  string
	Err      error
}

// Start loads or creates change id, adds a worktree leg per repo concurrently,
// and saves it when any leg exists. Repos sharing a bare name are rejected
// before anything is created, since worktrees are named by it.
func Start(ctx context.Context, store change.Store, id, title string, repos []workspace.Repo) (*change.Change, []AddResult, error) {
	if err := change.ValidateID(id); err != nil {
		return nil, nil, err
	}
	c, err := store.Load(id)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		c = &change.Change{ID: id, Branch: id, Created: time.Now().UTC()}
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
			res := &results[i]
			res.Leg, res.Warning, res.Err = addLeg(ctx, store.Dir(id), c.Branch, res.Repo)
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

func addLeg(ctx context.Context, dir, branch string, r workspace.Repo) (change.Leg, string, error) {
	var warn string
	if gitx.HasRemote(ctx, r.Path, "origin") {
		if err := gitx.Fetch(ctx, r.Path); err != nil {
			warn = "fetch failed, used local refs"
		}
	}
	base, err := gitx.DefaultBase(ctx, r.Path)
	if err != nil {
		return change.Leg{}, "", err
	}
	dst := filepath.Join(dir, filepath.Base(r.Name))
	if _, err := workspace.AddWorktree(ctx, r.Path, dst, branch, base); err != nil {
		return change.Leg{}, "", err
	}
	return change.Leg{Repo: r.Name, Source: r.Path, Worktree: dst, Base: base.Branch, BaseRef: base.Ref}, warn, nil
}

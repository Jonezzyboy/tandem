package core

import (
	"context"
	"sort"
	"sync"

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

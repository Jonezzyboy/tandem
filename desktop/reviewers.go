package main

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/gh"
)

const reviewersTTL = 10 * time.Minute

type reviewerCache struct {
	at   time.Time
	list []gh.Reviewer
}

// Reviewers suggests reviewers for the change's repos, cached for a while
// since the composer asks each time it opens.
func (a *App) Reviewers(id string) ([]gh.Reviewer, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	repos := make([]string, len(c.Legs))
	for i, l := range c.Legs {
		repos[i] = l.Repo
	}
	slices.Sort(repos)
	key := strings.Join(repos, ",")
	a.mu.Lock()
	cached, ok := a.reviewers[key]
	a.mu.Unlock()
	if ok && time.Since(cached.at) < reviewersTTL {
		return cached.list, nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	list, err := gh.Reviewers(ctx, repos)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.reviewers[key] = reviewerCache{at: time.Now(), list: list}
	a.mu.Unlock()
	return list, nil
}

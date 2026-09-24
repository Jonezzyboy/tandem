package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
)

type TrainOptions struct {
	Method string
	Poll   time.Duration
	// CheckTimeout bounds the wait for one leg's CI; MergeTimeout the wait
	// for a merge (a merge queue can hold it) to land.
	CheckTimeout time.Duration
	MergeTimeout time.Duration
	// Settle is how long a freshly pushed pin may show no checks before the
	// repo is taken to have no CI at all.
	Settle  time.Duration
	OnEvent func(TrainEvent)
}

func (o *TrainOptions) defaults() {
	if o.Method == "" {
		o.Method = "squash"
	}
	if o.Poll == 0 {
		o.Poll = 20 * time.Second
	}
	if o.CheckTimeout == 0 {
		o.CheckTimeout = 30 * time.Minute
	}
	if o.MergeTimeout == 0 {
		o.MergeTimeout = 15 * time.Minute
	}
	if o.Settle == 0 {
		o.Settle = 90 * time.Second
	}
	if o.OnEvent == nil {
		o.OnEvent = func(TrainEvent) {}
	}
}

type TrainEvent struct {
	Leg    string `json:"leg"`
	Level  int    `json:"level"`
	Phase  string `json:"phase"`
	Detail string `json:"detail"`
	URL    string `json:"url"`
}

type TrainCheck struct {
	Leg      *change.Leg
	Level    int
	PR       *gh.PR
	Merged   bool
	Problems []string
}

// TrainPreflight reports, per leg in merge order, what would stop a train.
// Failing checks are tolerated on legs the train will re-pin, since the pin
// is often what fixes them.
func TrainPreflight(ctx context.Context, c *change.Change, g Graph) []TrainCheck {
	states := Snapshot(ctx, c, true)
	SortByLevel(states, g.Levels)
	merged := map[string]bool{}
	for _, s := range states {
		if s.PR != nil && s.PR.State == "MERGED" {
			merged[s.Leg.Repo] = true
		}
	}
	repinned := map[string]bool{}
	for _, e := range goEdges(g) {
		if !merged[e.From] {
			repinned[e.To] = true
		}
	}
	out := make([]TrainCheck, 0, len(states))
	for _, s := range states {
		tc := TrainCheck{Leg: s.Leg, Level: g.Levels[s.Leg.Repo], PR: s.PR, Merged: merged[s.Leg.Repo]}
		switch {
		case s.PRErr != nil:
			tc.Problems = append(tc.Problems, FirstLine(s.PRErr.Error()))
		case s.PR == nil:
			tc.Problems = append(tc.Problems, "no PR")
		case tc.Merged:
		case s.PR.State != "OPEN":
			tc.Problems = append(tc.Problems, "PR closed without merging")
		default:
			tc.Problems = append(tc.Problems, readiness(s.PR, !repinned[s.Leg.Repo])...)
			if repinned[s.Leg.Repo] && !s.OnBranch() {
				tc.Problems = append(tc.Problems, "on "+orDetached(s.Status.Current)+", not "+c.Branch+": switch so it can be re-pinned")
			}
		}
		out = append(out, tc)
	}
	return out
}

func readiness(pr *gh.PR, checksCount bool) []string {
	var p []string
	if pr.IsDraft {
		p = append(p, "draft")
	}
	switch pr.ReviewDecision {
	case "CHANGES_REQUESTED":
		p = append(p, "changes requested")
	case "REVIEW_REQUIRED":
		p = append(p, "awaiting review")
	}
	if r := pr.Rollup(); checksCount && r.Fail > 0 {
		p = append(p, "failing checks: "+strings.Join(r.Failing, ", "))
	}
	return p
}

// RunTrain merges the change's PRs level by level. After a level lands, each
// downstream Go requirement on it is re-pinned to the merge commit and pushed,
// and that leg merges only once CI has passed on the new head. Legs already
// merged are skipped, so a stopped train resumes where it left off.
func RunTrain(ctx context.Context, c *change.Change, g Graph, o TrainOptions) error {
	o.defaults()
	checks := TrainPreflight(ctx, c, g)
	var problems []string
	for _, tc := range checks {
		for _, p := range tc.Problems {
			problems = append(problems, tc.Leg.Name()+": "+p)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("train cannot start:\n  %s", strings.Join(problems, "\n  "))
	}

	byLevel := map[int][]TrainCheck{}
	var levels []int
	for _, tc := range checks {
		if _, ok := byLevel[tc.Level]; !ok {
			levels = append(levels, tc.Level)
		}
		byLevel[tc.Level] = append(byLevel[tc.Level], tc)
	}
	sort.Ints(levels)

	mergedSHA := map[string]string{}
	expectHead := map[string]string{}
	for _, tc := range checks {
		if tc.Merged {
			mergedSHA[tc.Leg.Repo] = tc.PR.MergeSHA()
		}
	}
	for _, lvl := range levels {
		for _, tc := range byLevel[lvl] {
			l := tc.Leg
			if tc.Merged {
				o.OnEvent(TrainEvent{Leg: l.Name(), Level: lvl, Phase: "skipped", Detail: "already merged", URL: tc.PR.URL})
				continue
			}
			pr, err := waitReady(ctx, c, l, lvl, expectHead[l.Repo], o)
			if err != nil {
				return err
			}
			if pr.State != "MERGED" {
				o.OnEvent(TrainEvent{Leg: l.Name(), Level: lvl, Phase: "merging", Detail: "#" + strconv.Itoa(pr.Number) + " --" + o.Method, URL: pr.URL})
				if err := gh.Merge(ctx, l.Dir(), pr.Number, o.Method); err != nil {
					return fmt.Errorf("%s: %w", l.Name(), err)
				}
				if pr, err = waitMerged(ctx, c, l, o); err != nil {
					return err
				}
			}
			mergedSHA[l.Repo] = pr.MergeSHA()
			o.OnEvent(TrainEvent{Leg: l.Name(), Level: lvl, Phase: "merged", Detail: short(pr.MergeSHA()), URL: pr.URL})
		}
		if err := pinLevel(ctx, c, g, lvl, mergedSHA, expectHead, o); err != nil {
			return err
		}
	}
	o.OnEvent(TrainEvent{Phase: "done", Detail: "every leg merged"})
	return nil
}

// pinLevel re-points downstream legs at the merge commits of level lvl and
// pushes, recording the pushed head each downstream must show before it merges.
func pinLevel(ctx context.Context, c *change.Change, g Graph, lvl int, mergedSHA, expectHead map[string]string, o TrainOptions) error {
	for _, e := range goEdges(g) {
		if g.Levels[e.From] != lvl || mergedSHA[e.To] != "" {
			continue
		}
		sha := mergedSHA[e.From]
		if sha == "" {
			return fmt.Errorf("%s merged but GitHub reported no merge commit to pin to", e.From)
		}
		down, err := c.Leg(e.To)
		if err != nil {
			return err
		}
		changed, committed, err := PinTo(ctx, c, down, e.Dir, e.Via, sha, true, fmt.Sprintf("Pin %s to merged %s", e.Via, short(sha)))
		if err != nil {
			return err
		}
		if !changed {
			o.OnEvent(TrainEvent{Leg: down.Name(), Level: g.Levels[e.To], Phase: "pinned", Detail: e.Via + " already at " + short(sha)})
			continue
		}
		if committed {
			if _, err := gitx.Run(ctx, down.Dir(), "push", "--quiet", "origin", c.Branch); err != nil {
				return fmt.Errorf("%s: pinned %s but the push failed: %w", down.Name(), e.Via, err)
			}
			head, err := gitx.Run(ctx, down.Dir(), "rev-parse", "refs/heads/"+c.Branch)
			if err != nil {
				return err
			}
			expectHead[down.Repo] = head
		}
		o.OnEvent(TrainEvent{Leg: down.Name(), Level: g.Levels[e.To], Phase: "pinned", Detail: e.Via + " → " + short(sha) + ", pushed"})
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// waitReady polls until the PR can merge: open, on the expected head when one
// was just pushed, CI finished and green, approved (or no review required).
func waitReady(ctx context.Context, c *change.Change, l *change.Leg, lvl int, expectHead string, o TrainOptions) (*gh.PR, error) {
	start := time.Now()
	announced := false
	for {
		pr, err := ViewPR(ctx, c, l)
		switch {
		case err != nil:
			return nil, fmt.Errorf("%s: %w", l.Name(), err)
		case pr == nil:
			return nil, fmt.Errorf("%s: PR not found", l.Name())
		case pr.State == "MERGED":
			return pr, nil
		case pr.State != "OPEN":
			return nil, fmt.Errorf("%s: PR #%d was closed", l.Name(), pr.Number)
		}
		onHead := expectHead == "" || pr.HeadRefOid == expectHead
		r := pr.Rollup()
		if onHead && r.Fail > 0 {
			return nil, fmt.Errorf("%s: checks failed on #%d: %s", l.Name(), pr.Number, strings.Join(r.Failing, ", "))
		}
		settled := r.Pass+r.Pending > 0 || expectHead == "" || time.Since(start) > o.Settle
		if onHead && r.Pending == 0 && settled {
			if p := readiness(pr, true); len(p) > 0 {
				return nil, fmt.Errorf("%s: #%d is not ready: %s", l.Name(), pr.Number, strings.Join(p, ", "))
			}
			return pr, nil
		}
		if !announced {
			detail := fmt.Sprintf("waiting for CI on #%d", pr.Number)
			if !onHead {
				detail = fmt.Sprintf("waiting for GitHub to see the pinned push on #%d", pr.Number)
			}
			o.OnEvent(TrainEvent{Leg: l.Name(), Level: lvl, Phase: "waiting", Detail: detail, URL: pr.URL})
			announced = true
		}
		if time.Since(start) > o.CheckTimeout {
			return nil, fmt.Errorf("%s: gave up waiting for CI on #%d after %s", l.Name(), pr.Number, o.CheckTimeout)
		}
		if err := sleep(ctx, o.Poll); err != nil {
			return nil, err
		}
	}
}

func waitMerged(ctx context.Context, c *change.Change, l *change.Leg, o TrainOptions) (*gh.PR, error) {
	start := time.Now()
	for {
		pr, err := ViewPR(ctx, c, l)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", l.Name(), err)
		}
		if pr != nil && pr.State == "MERGED" {
			return pr, nil
		}
		if pr != nil && pr.State == "CLOSED" {
			return nil, fmt.Errorf("%s: PR #%d closed instead of merging", l.Name(), pr.Number)
		}
		if time.Since(start) > o.MergeTimeout {
			return nil, errors.New(l.Name() + ": merge did not land in time (a merge queue may still be holding it)")
		}
		if err := sleep(ctx, o.Poll); err != nil {
			return nil, err
		}
	}
}

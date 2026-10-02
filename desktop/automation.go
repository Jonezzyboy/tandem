package main

import (
	"context"
	"errors"
	"strings"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/triage"
)

type PinItem struct {
	Leg     string `json:"leg"`
	Module  string `json:"module"`
	Rev     string `json:"rev"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Pin re-points downstream Go legs at their upstream's merge commit (or pushed
// HEAD before it merges), commits and pushes.
func (a *App) Pin(id string) ([]PinItem, error) {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	g, err := core.BuildGraph(c)
	if err != nil {
		return nil, err
	}
	out := []PinItem{}
	for _, r := range core.Pin(a.ctx, c, g, true, true) {
		it := PinItem{Leg: r.Downstream.Name(), Module: r.Module, Rev: r.Rev}
		switch {
		case r.Skipped != "":
			it.Status, it.Message = "skipped", r.Skipped
		case r.Err != nil:
			it.Status, it.Message = "failed", core.FirstLine(r.Err.Error())
		case !r.Changed:
			it.Status, it.Message = "already", "already at "+short(r.Rev)
		default:
			it.Status, it.Message = "pinned", "pinned to "+short(r.Rev)+", pushed"
			if r.Merged {
				it.Message = "pinned to merged " + short(r.Rev) + ", pushed"
			}
		}
		out = append(out, it)
	}
	go a.refresh(id, false)
	return out, nil
}

// CommitPins commits and pushes the go.mod/go.sum updates for merged upstreams
// in every leg that has them, running any go get still outstanding first.
func (a *App) CommitPins(id string) ([]LegResult, error) {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	v, err := a.buildView(id, false)
	if err != nil {
		return nil, err
	}
	out := []LegResult{}
	for _, lv := range v.Legs {
		if len(lv.Pins) == 0 {
			continue
		}
		l, err := c.Leg(lv.Repo)
		if err != nil {
			return nil, err
		}
		mods := make([]string, len(lv.Pins))
		for i, p := range lv.Pins {
			mods[i] = p.Module
		}
		r := LegResult{Leg: lv.Name, OK: true, Message: "committed and pushed " + strings.Join(mods, ", ")}
		if err := core.CommitPins(a.ctx, c, l, lv.Pins); err != nil {
			r.OK, r.Message = false, core.FirstLine(err.Error())
		}
		out = append(out, r)
	}
	go a.refresh(id, true)
	return out, nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

type TrainLeg struct {
	Repo     string   `json:"repo"`
	Name     string   `json:"name"`
	Level    int      `json:"level"`
	PR       int      `json:"pr"`
	URL      string   `json:"url"`
	Merged   bool     `json:"merged"`
	Problems []string `json:"problems"`
}

type TrainPlan struct {
	Legs    []TrainLeg `json:"legs"`
	ToMerge int        `json:"toMerge"`
	Blocked int        `json:"blocked"`
	Running bool       `json:"running"`
}

func (a *App) TrainPlan(id string) (TrainPlan, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return TrainPlan{}, err
	}
	g, err := core.BuildGraph(c)
	if err != nil {
		return TrainPlan{}, err
	}
	p := TrainPlan{Legs: []TrainLeg{}}
	for _, tc := range core.TrainPreflight(a.ctx, c, g) {
		tl := TrainLeg{Repo: tc.Leg.Repo, Name: tc.Leg.Name(), Level: tc.Level, Merged: tc.Merged, Problems: tc.Problems}
		if tl.Problems == nil {
			tl.Problems = []string{}
		}
		if tc.PR != nil {
			tl.PR, tl.URL = tc.PR.Number, tc.PR.URL
		}
		switch {
		case len(tl.Problems) > 0:
			p.Blocked++
		case !tl.Merged:
			p.ToMerge++
		}
		p.Legs = append(p.Legs, tl)
	}
	a.mu.Lock()
	_, p.Running = a.trains[id]
	a.mu.Unlock()
	return p, nil
}

type TrainUpdate struct {
	Change string `json:"change"`
	core.TrainEvent
}

// StartTrain runs the merge train until it finishes, fails or is cancelled,
// streaming "train" events. Only one train runs per change.
func (a *App) StartTrain(id, method string) error {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	a.mu.Lock()
	if _, running := a.trains[id]; running {
		a.mu.Unlock()
		return errors.New("a merge train is already running for " + id)
	}
	a.trains[id] = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.trains, id)
		a.mu.Unlock()
	}()

	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return err
	}
	g, err := core.BuildGraph(c)
	if err != nil {
		return err
	}
	o := core.TrainOptions{Method: method, OnEvent: func(ev core.TrainEvent) {
		a.emit("train", TrainUpdate{Change: id, TrainEvent: ev})
	}}
	if t := a.Settings().Triage; t.Enabled {
		o.Triage, o.Retries = triage.New(t), t.Retries
	}
	err = core.RunTrain(ctx, c, g, o)
	if saveErr := a.store.Save(c); saveErr != nil && err == nil {
		err = saveErr
	}
	go a.refresh(id, true)
	if errors.Is(err, context.Canceled) {
		return errors.New("train stopped; merged legs stay merged, and running it again carries on")
	}
	return err
}

func (a *App) CancelTrain(id string) {
	a.mu.Lock()
	cancel := a.trains[id]
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

type CleanItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Ready     bool     `json:"ready"`
	Reason    string   `json:"reason"`
	Switches  []string `json:"switches"`
	Worktrees []string `json:"worktrees"`
	Branches  []string `json:"branches"`
	Kept      []string `json:"kept"`
	Files     []string `json:"files"`
	Dir       string   `json:"dir"`
	Warnings  []string `json:"warnings"`
}

// CleanPlan lists every change with exactly what cleaning it would remove.
func (a *App) CleanPlan() ([]CleanItem, error) {
	cands, err := core.CleanCandidates(a.ctx, a.store)
	if err != nil {
		return nil, err
	}
	out := make([]CleanItem, 0, len(cands))
	for _, cc := range cands {
		out = append(out, cleanItem(cc))
	}
	return out, nil
}

func cleanItem(cc core.CleanCandidate) CleanItem {
	it := CleanItem{ID: cc.Change.ID, Title: cc.Change.Title, Ready: cc.Ready, Reason: cc.Reason, Dir: cc.Dir,
		Switches: []string{}, Worktrees: []string{}, Branches: []string{}, Kept: []string{}, Files: []string{}, Warnings: []string{}}
	for _, s := range cc.Switches {
		it.Switches = append(it.Switches, s.Source+" → "+s.Base)
	}
	for _, w := range cc.Worktrees {
		it.Worktrees = append(it.Worktrees, w.Path)
	}
	for _, b := range cc.Branches {
		it.Branches = append(it.Branches, b.Branch+" in "+b.Source)
	}
	for _, b := range cc.Kept {
		it.Kept = append(it.Kept, b.Branch+" in "+b.Source)
	}
	it.Files = append(it.Files, cc.Files...)
	it.Warnings = append(it.Warnings, cc.Warnings...)
	return it
}

type CleanResult struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Clean removes the changes named in ids, re-checking each one first so that
// only a change still ready right now is touched.
func (a *App) Clean(ids []string) ([]CleanResult, error) {
	cands, err := core.CleanCandidates(a.ctx, a.store)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := []CleanResult{}
	for _, cc := range cands {
		if !want[cc.Change.ID] {
			continue
		}
		out = append(out, a.clean(cc, "cleaned"))
	}
	a.emit("changes", a.Changes())
	return out, nil
}

func (a *App) clean(cc core.CleanCandidate, done string) CleanResult {
	lock := a.opLock(cc.Change.ID)
	lock.Lock()
	err := core.Clean(a.ctx, cc)
	lock.Unlock()
	r := CleanResult{ID: cc.Change.ID, OK: err == nil, Message: done}
	if err != nil {
		r.Message = strings.ReplaceAll(err.Error(), "\n", "; ")
		return r
	}
	a.mu.Lock()
	delete(a.views, cc.Change.ID)
	if a.focus == cc.Change.ID {
		a.focus = ""
	}
	a.mu.Unlock()
	return r
}

// DiscardPlan lists what deleting change id now would remove, and the
// unlanded work that goes with it.
func (a *App) DiscardPlan(id string) (CleanItem, error) {
	cc, err := core.DiscardCandidate(a.ctx, a.store, id)
	if err != nil {
		return CleanItem{}, err
	}
	return cleanItem(cc), nil
}

// Discard deletes change id whether or not its work landed.
func (a *App) Discard(id string) (CleanResult, error) {
	cc, err := core.DiscardCandidate(a.ctx, a.store, id)
	if err != nil {
		return CleanResult{}, err
	}
	r := a.clean(cc, "deleted")
	a.emit("changes", a.Changes())
	return r, nil
}

// Switch checks out the change's branch in every leg (toBase: each leg's base
// branch), leaving legs with uncommitted work where they are.
func (a *App) Switch(id string, toBase bool) ([]LegResult, error) {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	out := []LegResult{}
	for _, r := range core.Switch(a.ctx, c, toBase) {
		lr := LegResult{Leg: r.Leg.Name(), OK: r.Err == nil, Message: "on " + r.To}
		if r.Skipped {
			lr.Message = "worktree, always on " + r.To
		}
		if r.Err != nil {
			lr.Message = "stayed put: " + core.FirstLine(r.Err.Error())
		}
		out = append(out, lr)
	}
	go a.refreshAll()
	return out, nil
}

// AddRepos puts more repos into an existing change, each on its branch.
func (a *App) AddRepos(id string, repos []string) ([]StartItem, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	return a.Start(StartRequest{ID: c.ID, Repos: repos})
}

// RemoveLeg takes a repo out of a change, leaving its branch in the repo.
func (a *App) RemoveLeg(id, leg string) (string, error) {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return "", err
	}
	l, err := c.RemoveLeg(leg)
	if err != nil {
		return "", err
	}
	if err := a.store.Save(c); err != nil {
		return "", err
	}
	a.emit("changes", a.Changes())
	go a.refresh(id, true)
	return l.Repo + " removed; branch " + c.Branch + " is left in " + l.Dir(), nil
}

// Unlink drops a declared merge-order edge.
func (a *App) Unlink(id, upstream, downstream string) error {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return err
	}
	up, err := c.Leg(upstream)
	if err != nil {
		return err
	}
	down, err := c.Leg(downstream)
	if err != nil {
		return err
	}
	if !c.RemoveDeclared(change.Edge{From: up.Repo, To: down.Repo}) {
		return errors.New(up.Name() + " → " + down.Name() + " comes from a manifest, not a declaration")
	}
	if err := a.store.Save(c); err != nil {
		return err
	}
	go a.refresh(id, false)
	return nil
}

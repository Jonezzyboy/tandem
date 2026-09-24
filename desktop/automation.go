package main

import (
	"context"
	"errors"
	"strings"

	"github.com/jonezzyboy/tandem/internal/core"
)

type PinItem struct {
	Leg     string `json:"leg"`
	Module  string `json:"module"`
	Rev     string `json:"rev"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Pin re-points downstream Go legs at their upstream's pushed HEAD and commits.
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
	for _, r := range core.Pin(a.ctx, c, g, true) {
		it := PinItem{Leg: r.Downstream.Name(), Module: r.Module, Rev: r.Rev}
		switch {
		case r.Skipped != "":
			it.Status, it.Message = "skipped", r.Skipped
		case r.Err != nil:
			it.Status, it.Message = "failed", core.FirstLine(r.Err.Error())
		case !r.Changed:
			it.Status, it.Message = "already", "already at "+short(r.Rev)
		default:
			it.Status, it.Message = "pinned", "pinned to "+short(r.Rev)+", committed"
		}
		out = append(out, it)
	}
	go a.refresh(id, false)
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
	err = core.RunTrain(ctx, c, g, core.TrainOptions{Method: method, OnEvent: func(ev core.TrainEvent) {
		a.emit("train", TrainUpdate{Change: id, TrainEvent: ev})
	}})
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
	Worktrees []string `json:"worktrees"`
	Branches  []string `json:"branches"`
	Kept      []string `json:"kept"`
	Files     []string `json:"files"`
	Dir       string   `json:"dir"`
}

// CleanPlan lists every change with exactly what cleaning it would remove.
func (a *App) CleanPlan() ([]CleanItem, error) {
	cands, err := core.CleanCandidates(a.ctx, a.store)
	if err != nil {
		return nil, err
	}
	out := make([]CleanItem, 0, len(cands))
	for _, cc := range cands {
		it := CleanItem{ID: cc.Change.ID, Title: cc.Change.Title, Ready: cc.Ready, Reason: cc.Reason, Dir: cc.Dir,
			Worktrees: []string{}, Branches: []string{}, Kept: []string{}, Files: []string{}}
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
		out = append(out, it)
	}
	return out, nil
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
		lock := a.opLock(cc.Change.ID)
		lock.Lock()
		err := core.Clean(a.ctx, cc)
		lock.Unlock()
		r := CleanResult{ID: cc.Change.ID, OK: err == nil, Message: "cleaned"}
		if err != nil {
			r.Message = strings.ReplaceAll(err.Error(), "\n", "; ")
		} else {
			a.mu.Lock()
			delete(a.views, cc.Change.ID)
			a.mu.Unlock()
		}
		out = append(out, r)
	}
	a.emit("changes", a.Changes())
	return out, nil
}

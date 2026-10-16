package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gitx"
)

const syncUndoFile = "sync-undo.json"

type SyncPreview struct {
	Leg      string   `json:"leg"`
	Commands []string `json:"commands"`
	Skip     string   `json:"skip"`
}

// SyncPlan fetches every leg and lists the git commands Sync would then run.
func (a *App) SyncPlan(id string) ([]SyncPreview, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	g, _ := core.BuildGraph(c)
	steps := core.PlanSync(a.ctx, c, core.OrderedLegs(c, g.Levels))
	out := make([]SyncPreview, len(steps))
	for i, s := range steps {
		out[i] = SyncPreview{Leg: s.Leg.Name(), Commands: s.Commands, Skip: s.Skip}
	}
	return out, nil
}

// rebased is one leg's commit before and after a sync's rebase.
type rebased struct {
	Leg    string `json:"leg"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type syncUndo struct {
	At   time.Time `json:"at"`
	Legs []rebased `json:"legs"`
}

func (a *App) saveSyncUndo(id string, results []core.SyncResult) {
	u := syncUndo{At: time.Now()}
	for _, r := range results {
		if r.Before != "" && r.After != "" && r.Before != r.After {
			u.Legs = append(u.Legs, rebased{Leg: r.Leg.Name(), Before: r.Before, After: r.After})
		}
	}
	path := filepath.Join(a.store.Dir(id), syncUndoFile)
	if len(u.Legs) == 0 {
		os.Remove(path)
		return
	}
	if data, err := json.Marshal(u); err == nil {
		os.WriteFile(path, data, 0o644)
	}
}

func (a *App) dropSyncUndo(id string) {
	os.Remove(filepath.Join(a.store.Dir(id), syncUndoFile))
}

// CanUndoSync reports which legs the last sync rebased, while it can still be
// undone: until the next publish.
func (a *App) CanUndoSync(id string) []string {
	var u syncUndo
	data, err := os.ReadFile(filepath.Join(a.store.Dir(id), syncUndoFile))
	if err != nil || json.Unmarshal(data, &u) != nil {
		return []string{}
	}
	out := make([]string, len(u.Legs))
	for i, l := range u.Legs {
		out[i] = l.Leg
	}
	return out
}

// UndoSync puts each leg the last sync rebased back on its old commit, with
// git reset --keep so nothing uncommitted is lost. A leg that has moved on
// since, by a new commit or a switch, is left alone.
func (a *App) UndoSync(id string) ([]LegResult, error) {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	var u syncUndo
	data, err := os.ReadFile(filepath.Join(a.store.Dir(id), syncUndoFile))
	if err != nil || json.Unmarshal(data, &u) != nil || len(u.Legs) == 0 {
		return nil, errors.New("there's no sync to undo")
	}
	var out []LegResult
	for _, r := range u.Legs {
		out = append(out, a.undoLeg(c, r))
	}
	a.dropSyncUndo(id)
	go a.refresh(id, false)
	return out, nil
}

func (a *App) undoLeg(c *change.Change, r rebased) LegResult {
	l, err := c.Leg(r.Leg)
	if err != nil {
		return LegResult{Leg: r.Leg, Message: err.Error()}
	}
	dir := l.Dir()
	if cur := gitx.CurrentBranch(a.ctx, dir); cur != c.Branch {
		return LegResult{Leg: r.Leg, Message: fmt.Sprintf("left alone: on %s now, not %s", cur, c.Branch)}
	}
	if head, _ := gitx.Run(a.ctx, dir, "rev-parse", "HEAD"); head != r.After {
		return LegResult{Leg: r.Leg, Message: "left alone: it has new commits since the sync"}
	}
	if _, err := gitx.Run(a.ctx, dir, "reset", "--keep", r.Before); err != nil {
		return LegResult{Leg: r.Leg, Message: core.FirstLine(err.Error())}
	}
	return LegResult{Leg: r.Leg, OK: true, Message: "back on " + short(r.Before) + ", as before the sync"}
}

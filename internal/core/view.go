package core

import (
	"path/filepath"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/graph"
)

// ChangeView is a change's state flattened for rendering or JSON.
type ChangeView struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Branch     string     `json:"branch"`
	Body       string     `json:"body"`
	Reviewers  []string   `json:"reviewers"`
	Legs       []LegView  `json:"legs"`
	Edges      []EdgeView `json:"edges"`
	GraphError string     `json:"graphError,omitempty"`
	Blocked    int        `json:"blocked"`
	// Remote is false until GitHub state has been read at least once.
	Remote    bool      `json:"remote"`
	RemoteAt  time.Time `json:"remoteAt"`
	CheckedAt time.Time `json:"checkedAt"`
}

type EdgeView struct {
	From string `json:"from"`
	To   string `json:"to"`
	Via  string `json:"via,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type LegView struct {
	Repo       string   `json:"repo"`
	Name       string   `json:"name"`
	Dir        string   `json:"dir"`
	Lang       string   `json:"lang"`
	Current    string   `json:"current"`
	OnBranch   bool     `json:"onBranch"`
	Base       string   `json:"base"`
	BaseRef    string   `json:"baseRef"`
	Level      int      `json:"level"`
	Ahead      int      `json:"ahead"`
	Behind     int      `json:"behind"`
	Dirty      int      `json:"dirty"`
	LocalError string   `json:"localError,omitempty"`
	PR         *PRView  `json:"pr"`
	PRError    string   `json:"prError,omitempty"`
	Blockers   []string `json:"blockers"`
}

type PRView struct {
	Number  int      `json:"number"`
	URL     string   `json:"url"`
	State   string   `json:"state"`
	Draft   bool     `json:"draft"`
	Review  string   `json:"review"`
	Pass    int      `json:"pass"`
	Fail    int      `json:"fail"`
	Pending int      `json:"pending"`
	Failing []string `json:"failing"`
}

func NewPRView(pr *gh.PR) *PRView {
	if pr == nil {
		return nil
	}
	r := pr.Rollup()
	return &PRView{
		Number: pr.Number, URL: pr.URL, State: pr.State, Draft: pr.IsDraft, Review: pr.ReviewDecision,
		Pass: r.Pass, Fail: r.Fail, Pending: r.Pending, Failing: r.Failing,
	}
}

// BuildView renders states in merge order. With remote false, PR fields and
// blockers are left empty for the caller to carry over from an earlier view.
func BuildView(c *change.Change, g Graph, graphErr error, states []LegState, remote bool) ChangeView {
	SortByLevel(states, g.Levels)
	v := ChangeView{
		ID: c.ID, Title: c.Title, Branch: c.Branch, Body: c.Body, Reviewers: c.Reviewers,
		Legs: make([]LegView, 0, len(states)), Edges: []EdgeView{}, Remote: remote, CheckedAt: time.Now(),
	}
	if remote {
		v.RemoteAt = v.CheckedAt
	}
	if graphErr != nil {
		v.GraphError = graphErr.Error()
	}
	for _, e := range g.Edges {
		v.Edges = append(v.Edges, EdgeView{From: e.From, To: e.To, Via: e.Via, Kind: string(e.Kind)})
	}
	for _, s := range states {
		lv := LegView{
			Repo: s.Leg.Repo, Name: filepath.Base(s.Leg.Repo), Dir: s.Leg.Dir(), Lang: graph.Language(s.Leg.Dir()),
			Current: s.Status.Current, OnBranch: s.OnBranch(),
			Base: s.Leg.Base, BaseRef: s.Leg.BaseRef, Level: g.Levels[s.Leg.Repo],
			Ahead: s.Status.Ahead, Behind: s.Status.Behind, Dirty: s.Status.Dirty, Blockers: []string{},
		}
		if s.StatusErr != nil {
			lv.LocalError = FirstLine(s.StatusErr.Error())
		}
		if remote {
			lv.PR = NewPRView(s.PR)
			if s.PRErr != nil {
				lv.PRError = FirstLine(s.PRErr.Error())
			}
			lv.Blockers = append(lv.Blockers, s.Blockers()...)
			if len(lv.Blockers) > 0 {
				v.Blocked++
			}
		}
		v.Legs = append(v.Legs, lv)
	}
	return v
}

// MergeLocal returns next (a local-only view) with the GitHub state of prev
// carried over, so a quick local refresh never blanks PR columns.
func MergeLocal(prev, next ChangeView) ChangeView {
	if !prev.Remote {
		return next
	}
	byRepo := make(map[string]LegView, len(prev.Legs))
	for _, l := range prev.Legs {
		byRepo[l.Repo] = l
	}
	next.Remote, next.RemoteAt = true, prev.RemoteAt
	next.Blocked = 0
	for i, l := range next.Legs {
		old, ok := byRepo[l.Repo]
		if !ok {
			continue
		}
		l.PR, l.PRError = old.PR, old.PRError
		l.Blockers = []string{}
		for _, b := range old.Blockers {
			if b != "uncommitted changes" {
				l.Blockers = append(l.Blockers, b)
			}
		}
		if l.Dirty > 0 && l.OnBranch && l.PR != nil && l.PR.State == "OPEN" {
			l.Blockers = append(l.Blockers, "uncommitted changes")
		}
		if len(l.Blockers) > 0 {
			next.Blocked++
		}
		next.Legs[i] = l
	}
	return next
}

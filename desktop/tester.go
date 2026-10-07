package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/jira"
	"github.com/jonezzyboy/tandem/internal/testrun"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

// Tester mode finds changes to test from Jira (tickets in the ready status)
// and GitHub (open PRs whose head branch is the ticket key), so a tester needs
// nothing a developer recorded locally.

const queueTTL = time.Minute

type TestPR struct {
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	URL     string `json:"url"`
	Draft   bool   `json:"draft"`
	Author  string `json:"author"`
	Updated string `json:"updated"`
	Pass    int    `json:"pass"`
	Fail    int    `json:"fail"`
	Pending int    `json:"pending"`
}

type QueueItem struct {
	Key      string   `json:"key"`
	URL      string   `json:"url"`
	Summary  string   `json:"summary"`
	Status   string   `json:"status"`
	Assignee string   `json:"assignee"`
	PRs      []TestPR `json:"prs"`
}

type TesterQueue struct {
	Items []QueueItem `json:"items"`
	// Setup names what's missing before the queue can be read; Error is any
	// other failure.
	Setup string `json:"setup"`
	Error string `json:"error"`
}

func (a *App) jiraClient() jira.Client {
	email := a.Settings().Jira.Email
	return jira.Client{Email: email, Token: jiraToken(email)}
}

// owners are the GitHub owners cloned under the roots: PR searches are limited
// to them so same-named branches elsewhere on GitHub don't show up.
func (a *App) owners() map[string]bool {
	out := map[string]bool{}
	for _, r := range workspace.List(a.roots) {
		owner, _, _ := strings.Cut(r.Name, "/")
		out[strings.ToLower(owner)] = true
	}
	return out
}

// prsFor finds key's open PRs in the given owners, with their CI.
func (a *App) prsFor(ctx context.Context, key string, owners map[string]bool) ([]TestPR, error) {
	found, err := gh.SearchHead(ctx, key, nil)
	if err != nil {
		return nil, err
	}
	var out []TestPR
	for _, p := range found {
		owner, _, _ := strings.Cut(p.Repository.NameWithOwner, "/")
		if owners[strings.ToLower(owner)] {
			out = append(out, TestPR{Repo: p.Repository.NameWithOwner, Number: p.Number, URL: p.URL, Draft: p.IsDraft,
				Author: p.Author.Login, Updated: p.UpdatedAt.Format(time.RFC3339)})
		}
	}
	var wg sync.WaitGroup
	for i := range out {
		wg.Go(func() {
			if pr, err := gh.ViewIn(ctx, out[i].Repo, out[i].Number); err == nil {
				r := pr.Rollup()
				out[i].Pass, out[i].Fail, out[i].Pending = r.Pass, r.Fail, r.Pending
			}
		})
	}
	wg.Wait()
	slices.SortFunc(out, func(x, y TestPR) int { return strings.Compare(x.Repo, y.Repo) })
	return out, nil
}

// TesterQueue lists tickets in the ready-to-test status with their PRs. It is
// cached for a minute unless force is set.
func (a *App) TesterQueue(force bool) TesterQueue {
	a.mu.Lock()
	if !force && a.queue != nil && time.Since(a.queueAt) < queueTTL {
		q := *a.queue
		a.mu.Unlock()
		return q
	}
	a.mu.Unlock()
	q := a.fetchQueue()
	a.mu.Lock()
	a.queue, a.queueAt = &q, time.Now()
	a.mu.Unlock()
	return q
}

func (a *App) fetchQueue() TesterQueue {
	s := a.Settings()
	q := TesterQueue{Items: []QueueItem{}}
	switch {
	case s.Jira.Site == "":
		q.Setup = "Add your Jira site in Settings to see tickets ready to test."
		return q
	case s.Jira.Email == "" || jiraToken(s.Jira.Email) == "":
		q.Setup = "Connect Jira in Settings to see tickets ready to test."
		return q
	}
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	defer cancel()
	issues, err := a.jiraClient().Search(ctx, s.Jira.Site, fmt.Sprintf("status = %q ORDER BY updated DESC", s.Tester.ReadyStatus))
	if err != nil {
		q.Error = err.Error()
		return q
	}
	owners := a.owners()
	q.Items = make([]QueueItem, len(issues))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, is := range issues {
		q.Items[i] = QueueItem{Key: is.Key, URL: jira.Ref{Site: s.Jira.Site, Key: is.Key}.URL(), Summary: is.Summary, Status: is.Status, Assignee: is.Assignee, PRs: []TestPR{}}
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if prs, err := a.prsFor(ctx, is.Key, owners); err == nil {
				q.Items[i].PRs = prs
			}
		})
	}
	wg.Wait()
	return q
}

type PlanRepo struct {
	Name    string  `json:"name"`
	Cloned  bool    `json:"cloned"`
	Current string  `json:"current"`
	Dirty   int     `json:"dirty"`
	PR      *TestPR `json:"pr"`
}

type TesterPlan struct {
	Ticket    JiraTicket `json:"ticket"`
	Repos     []PlanRepo `json:"repos"`
	CloneRoot string     `json:"cloneRoot"`
	Error     string     `json:"error"`
}

// TesterPlan gathers what testing key would touch: its ticket, and every repo
// with its branch, from open PRs and from clones that have it on origin.
func (a *App) TesterPlan(key string) (TesterPlan, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if r, ok := jira.Parse(key); ok {
		key = r.Key
	}
	if !jira.IsKey(key) {
		return TesterPlan{}, fmt.Errorf("%q isn't a Jira ticket key or link", key)
	}
	s := a.Settings()
	p := TesterPlan{Repos: []PlanRepo{}, CloneRoot: a.cloneRoot()}
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	defer cancel()
	var prs []TestPR
	var prErr error
	var local []core.BranchRepo
	var wg sync.WaitGroup
	wg.Go(func() { prs, prErr = a.prsFor(ctx, key, a.owners()) })
	wg.Go(func() { local = core.FindBranch(ctx, workspace.List(a.roots), key) })
	if s.Jira.Site != "" {
		wg.Go(func() {
			p.Ticket, _ = a.JiraLookup(jira.Ref{Site: s.Jira.Site, Key: key}.URL())
		})
	}
	wg.Wait()
	if p.Ticket.Key == "" {
		p.Ticket = JiraTicket{Key: key, Error: "Add your Jira site in Settings to see the ticket."}
	}
	if prErr != nil {
		p.Error = "Couldn't search GitHub: " + core.FirstLine(prErr.Error())
	}
	byName := map[string]*PlanRepo{}
	add := func(name string) *PlanRepo {
		if r := byName[strings.ToLower(name)]; r != nil {
			return r
		}
		r := &PlanRepo{Name: name}
		byName[strings.ToLower(name)] = r
		return r
	}
	for _, pr := range prs {
		add(pr.Repo).PR = &pr
	}
	for _, b := range local {
		if b.Remote {
			add(b.Repo.Name)
		}
	}
	for _, r := range byName {
		if repo, err := workspace.Resolve(a.roots, r.Name); err == nil {
			r.Cloned = true
			r.Current = gitx.CurrentBranch(ctx, repo.Path)
			if st, err := gitx.Run(ctx, repo.Path, "status", "--porcelain", "--untracked-files=no"); err == nil && st != "" {
				r.Dirty = strings.Count(st, "\n") + 1
			}
		}
		p.Repos = append(p.Repos, *r)
	}
	slices.SortFunc(p.Repos, func(x, y PlanRepo) int { return strings.Compare(x.Name, y.Name) })
	return p, nil
}

func (a *App) cloneRoot() string {
	if !a.Settings().Tester.CloneMissing || len(a.roots) == 0 {
		return ""
	}
	return a.roots[0]
}

func (a *App) testStore() testrun.Store {
	return testrun.Store{Path: filepath.Join(a.store.Home, "tester.json")}
}

// TesterState is the change under test, if any, and what was tested before.
func (a *App) TesterState() (testrun.State, error) {
	st, err := a.testStore().Load()
	if st.History == nil {
		st.History = []testrun.Record{}
	}
	return st, err
}

type TestStartRequest struct {
	Key      string   `json:"key"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Repos    []string `json:"repos"`
	SetAside bool     `json:"setAside"`
}

// TesterStart switches every repo in req to the change's branch, emitting
// "tester-step" as each repo progresses. Testing one change at a time keeps
// "back to main" simple, so another under test is refused.
func (a *App) TesterStart(req TestStartRequest) ([]testrun.Step, error) {
	a.testMu.Lock()
	defer a.testMu.Unlock()
	st, err := a.testStore().Load()
	if err != nil {
		return nil, err
	}
	if st.Current != nil && st.Current.Key != req.Key {
		return nil, fmt.Errorf("you're testing %s: go back to main first", st.Current.Key)
	}
	if len(req.Repos) == 0 {
		return nil, errors.New("no repos have this change's branch")
	}
	targets := make([]testrun.Target, len(req.Repos))
	for i, name := range req.Repos {
		targets[i] = testrun.Target{Name: name}
		if r, err := workspace.Resolve(a.roots, name); err == nil {
			targets[i].Dir = r.Path
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	s, steps := testrun.Begin(ctx, req.Key, req.Title, req.URL, targets, testrun.Options{
		SetAside: req.SetAside, CloneRoot: a.cloneRoot(),
		OnStep: func(s testrun.Step) { a.emit("tester-step", s) },
	})
	if len(s.Repos) > 0 {
		if st.Current != nil {
			s.Started = st.Current.Started
			for _, r := range st.Current.Repos {
				if !slices.ContainsFunc(s.Repos, func(x testrun.Repo) bool { return x.Name == r.Name }) {
					s.Repos = append(s.Repos, r)
				}
			}
		}
		st.Current = s
		if err := a.testStore().Save(st); err != nil {
			return steps, err
		}
	}
	return steps, nil
}

// TesterStatus fetches the repos under test and compares them with origin.
func (a *App) TesterStatus() ([]testrun.RepoStatus, error) {
	st, err := a.testStore().Load()
	if err != nil || st.Current == nil {
		return []testrun.RepoStatus{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	defer cancel()
	return testrun.Status(ctx, st.Current), nil
}

// TesterPull brings every repo under test up to origin's latest.
func (a *App) TesterPull() ([]testrun.Step, error) {
	a.testMu.Lock()
	defer a.testMu.Unlock()
	st, err := a.testStore().Load()
	if err != nil {
		return nil, err
	}
	if st.Current == nil {
		return nil, errors.New("nothing is being tested")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return testrun.Pull(ctx, st.Current), nil
}

type TestFinish struct {
	Steps []testrun.Step `json:"steps"`
	// Jira says what happened to the ticket, JiraError why it didn't.
	Jira      string `json:"jira"`
	JiraError string `json:"jiraError"`
	Done      bool   `json:"done"`
}

// TesterFinish records result ("passed", "failed", or "" to just stop) on the
// ticket with note as a comment, then puts every repo back on its base. Repos
// that can't go back stay in the session for another try.
func (a *App) TesterFinish(result, note string) (TestFinish, error) {
	a.testMu.Lock()
	defer a.testMu.Unlock()
	st, err := a.testStore().Load()
	if err != nil {
		return TestFinish{}, err
	}
	if st.Current == nil {
		return TestFinish{}, errors.New("nothing is being tested")
	}
	s := st.Current
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	out := TestFinish{}
	if result != "" {
		out.Jira, out.JiraError = a.recordResult(ctx, s, result, note)
	}
	steps, left := testrun.End(ctx, s)
	out.Steps = steps
	if len(left) == 0 {
		out.Done = true
		st.Current = nil
		st.History = append([]testrun.Record{{Key: s.Key, Title: s.Title, URL: s.URL, Result: cmp.Or(result, "stopped"), At: time.Now().UTC()}}, st.History...)
	} else {
		s.Repos = left
	}
	if err := a.testStore().Save(st); err != nil {
		return out, err
	}
	a.mu.Lock()
	a.queue = nil
	a.mu.Unlock()
	return out, nil
}

func (a *App) recordResult(ctx context.Context, s *testrun.Session, result, note string) (string, string) {
	ref, ok := jira.Parse(s.URL)
	if !ok {
		return "", "no Jira link to update"
	}
	t := a.Settings().Tester
	to := t.PassStatus
	if result == "failed" {
		to = t.FailStatus
	}
	c := a.jiraClient()
	if err := c.MoveTo(ctx, ref, to); err != nil {
		return "", err.Error()
	}
	if strings.TrimSpace(note) != "" {
		if err := c.Comment(ctx, ref, note); err != nil {
			return "moved to " + to, "the note wasn't added: " + err.Error()
		}
		return "moved to " + to + ", with your note", ""
	}
	return "moved to " + to, ""
}

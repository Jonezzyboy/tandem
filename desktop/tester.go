package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
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
	Branch  string `json:"branch"`
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
	Type     string   `json:"type"`
	Priority string   `json:"priority"`
	Assignee string   `json:"assignee"`
	Updated  string   `json:"updated"`
	// StatusSince is when the ticket moved into Status, or empty when Jira
	// didn't say.
	StatusSince string   `json:"statusSince"`
	PRs         []TestPR `json:"prs"`
	// PRsLoaded is false until GitHub has been searched for the ticket's PRs.
	PRsLoaded bool `json:"prsLoaded"`
}

type TesterQueue struct {
	Items    []QueueItem `json:"items"`
	Statuses []string    `json:"statuses"`
	// Setup names what's missing before the queue can be read; Error is any
	// other failure.
	Setup string `json:"setup"`
	Error string `json:"error"`
	// At is when Jira was read; Loading is true while PRs are still being found.
	At      string `json:"at"`
	Loading bool   `json:"loading"`
	PRError string `json:"prError"`
}

func (a *App) jiraClient() jira.Client {
	email := a.Settings().Jira.Email
	return jira.Client{Email: email, Token: jiraToken(email)}
}

// owners are the GitHub owners cloned under the roots: PRs are looked for in
// them only, so same-named branches elsewhere on GitHub don't show up.
func (a *App) owners() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range workspace.List(a.roots) {
		owner, _, _ := strings.Cut(r.Name, "/")
		if o := strings.ToLower(owner); !seen[o] {
			seen[o] = true
			out = append(out, owner)
		}
	}
	return out
}

const openPRsTTL = time.Minute

// openPRs lists the open PRs in every owner, one query per owner (a query
// naming an owner GitHub doesn't know finds nothing at all), cached for a
// minute. It fails only when no owner could be read.
func (a *App) openPRs(ctx context.Context) ([]gh.OpenPR, error) {
	a.mu.Lock()
	if a.prs != nil && time.Since(a.prsAt) < openPRsTTL {
		prs := a.prs
		a.mu.Unlock()
		return prs, nil
	}
	a.mu.Unlock()
	owners := a.owners()
	found := make([][]gh.OpenPR, len(owners))
	errs := make([]error, len(owners))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, o := range owners {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			found[i], errs[i] = gh.OpenPRs(ctx, o)
		})
	}
	wg.Wait()
	all := []gh.OpenPR{}
	failed := 0
	for i := range owners {
		all = append(all, found[i]...)
		if errs[i] != nil {
			failed++
		}
	}
	if failed > 0 && failed == len(owners) {
		return nil, errs[0]
	}
	a.mu.Lock()
	a.prs, a.prsAt = all, time.Now()
	a.mu.Unlock()
	return all, nil
}

// mentions reports whether s names key on its own: DEV-12 in "DEV-12",
// "DEV-12-fix-prices" or "feature/DEV-12", not in "DEV-120".
func mentions(s, key string) bool {
	s, key = strings.ToUpper(s), strings.ToUpper(key)
	for i := 0; ; {
		j := strings.Index(s[i:], key)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(key)
		before := start == 0 || !isWordByte(s[start-1])
		after := end == len(s) || !(s[end] >= '0' && s[end] <= '9')
		if before && after {
			return true
		}
		i = start + 1
	}
}

func isWordByte(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

// prsFor picks key's PRs out of all: those whose branch or title names it.
func prsFor(all []gh.OpenPR, key string) []TestPR {
	out := []TestPR{}
	for _, p := range all {
		if !mentions(p.Branch, key) && !mentions(p.Title, key) {
			continue
		}
		t := TestPR{Repo: p.Repo, Branch: p.Branch, Number: p.Number, URL: p.URL, Draft: p.Draft, Author: p.Author, Updated: p.Updated.Format(time.RFC3339)}
		switch p.CI {
		case "SUCCESS":
			t.Pass = 1
		case "FAILURE", "ERROR":
			t.Fail = 1
		case "PENDING", "EXPECTED":
			t.Pending = 1
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(x, y TestPR) int { return strings.Compare(x.Repo, y.Repo) })
	return out
}

// TesterQueue lists tickets in the statuses to test. Jira answers quickly, so
// it returns those at once and finds their PRs in the background, emitting
// "tester-queue" as each ticket's arrive. It is cached for a minute unless
// force is set.
func (a *App) TesterQueue(force bool) TesterQueue {
	a.mu.Lock()
	if !force && a.queue != nil && (a.queue.Loading || time.Since(a.queueAt) < queueTTL) {
		q := cloneQueue(*a.queue)
		a.mu.Unlock()
		return q
	}
	a.queueGen++
	gen := a.queueGen
	a.mu.Unlock()
	q := a.readTickets()
	q.Loading = len(q.Items) > 0
	a.mu.Lock()
	if gen == a.queueGen {
		a.queue, a.queueAt = &q, time.Now()
	}
	a.mu.Unlock()
	if q.Loading {
		go a.fillPRs(gen)
	}
	return cloneQueue(q)
}

func cloneQueue(q TesterQueue) TesterQueue {
	q.Items = slices.Clone(q.Items)
	return q
}

func (a *App) readTickets() TesterQueue {
	s := a.Settings()
	q := TesterQueue{Items: []QueueItem{}, Statuses: s.Tester.Statuses, At: time.Now().Format(time.RFC3339)}
	switch {
	case s.Jira.Site == "":
		q.Setup = "Add your Jira site in Settings to see tickets ready to test."
		return q
	case s.Jira.Email == "" || jiraToken(s.Jira.Email) == "":
		q.Setup = "Connect Jira in Settings to see tickets ready to test."
		return q
	case len(s.Tester.Statuses) == 0:
		q.Setup = "Choose which Jira statuses mean a ticket is ready to test, in Settings."
		return q
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	issues, err := a.jiraClient().Search(ctx, s.Jira.Site, jira.StatusJQL(s.Tester.Statuses))
	if err != nil {
		q.Error = err.Error()
		return q
	}
	for _, is := range issues {
		it := QueueItem{Key: is.Key, URL: jira.Ref{Site: s.Jira.Site, Key: is.Key}.URL(), Summary: is.Summary, Status: is.Status,
			Type: is.Type, Priority: is.Priority, Assignee: is.Assignee, PRs: []TestPR{}}
		if !is.Updated.IsZero() {
			it.Updated = is.Updated.Format(time.RFC3339)
		}
		if !is.StatusSince.IsZero() {
			it.StatusSince = is.StatusSince.Format(time.RFC3339)
		}
		q.Items = append(q.Items, it)
	}
	return q
}

// fillPRs matches each ticket to its PRs and folds them into the cached
// queue gen, unless a newer read has replaced it.
func (a *App) fillPRs(gen int) {
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	all, err := a.openPRs(ctx)
	a.updateQueue(gen, func(q *TesterQueue) {
		q.Loading = false
		if err != nil {
			q.PRError = "Couldn't read PRs from GitHub: " + core.FirstLine(err.Error())
			return
		}
		for i := range q.Items {
			q.Items[i].PRs, q.Items[i].PRsLoaded = prsFor(all, q.Items[i].Key), true
		}
	})
}

func (a *App) updateQueue(gen int, fn func(*TesterQueue)) {
	a.mu.Lock()
	if gen != a.queueGen || a.queue == nil {
		a.mu.Unlock()
		return
	}
	q := cloneQueue(*a.queue)
	fn(&q)
	a.queue = &q
	out := cloneQueue(q)
	a.mu.Unlock()
	a.emit("tester-queue", out)
}

// JiraStatuses lists the statuses on the Jira site, for choosing which to test.
func (a *App) JiraStatuses() ([]string, error) {
	site := a.Settings().Jira.Site
	if site == "" {
		return []string{}, errors.New("add your Jira site first")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return a.jiraClient().Statuses(ctx, site)
}

type PlanRepo struct {
	Name string `json:"name"`
	// Branch is the change's branch in this repo.
	Branch  string  `json:"branch"`
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
	// Checklist is what to try, from the ticket's acceptance criteria.
	Checklist []string `json:"checklist"`
	// Last is the previous test of this ticket on this machine, and Changes
	// what was pushed to each of its repos since.
	Last    *testrun.Record `json:"last"`
	Changes []RepoChanges   `json:"changes"`
}

type RepoChanges struct {
	Repo    string           `json:"repo"`
	Commits []testrun.Commit `json:"commits"`
	Error   string           `json:"error"`
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
	p := TesterPlan{Repos: []PlanRepo{}, CloneRoot: a.cloneRoot(), Checklist: []string{}, Changes: []RepoChanges{}}
	if st, err := a.testStore().Load(); err == nil {
		p.Last = st.Last(key)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	defer cancel()
	var prs []TestPR
	var prErr error
	var local []core.BranchRepo
	var wg sync.WaitGroup
	wg.Go(func() {
		all, err := a.openPRs(ctx)
		prs, prErr = prsFor(all, key), err
	})
	wg.Go(func() { local = core.FindBranch(ctx, workspace.List(a.roots), key) })
	if s.Jira.Site != "" {
		wg.Go(func() {
			p.Ticket, _ = a.JiraLookup(jira.Ref{Site: s.Jira.Site, Key: key}.URL())
		})
	}
	if p.Last != nil {
		p.Changes = make([]RepoChanges, len(p.Last.Repos))
		for i, t := range p.Last.Repos {
			wg.Go(func() {
				c := RepoChanges{Repo: t.Name, Commits: []testrun.Commit{}}
				if repo, err := workspace.Resolve(a.roots, t.Name); err != nil {
					c.Error = "not cloned on this machine"
				} else if c.Commits, err = testrun.Since(ctx, repo.Path, t.Branch, t.SHA); err != nil {
					c.Error = core.FirstLine(err.Error())
				}
				p.Changes[i] = c
			})
		}
	}
	wg.Wait()
	p.Checklist = checklist(p.Ticket.Description)
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
		r := add(pr.Repo)
		r.PR, r.Branch = &pr, pr.Branch
	}
	for _, b := range local {
		if b.Remote {
			if r := add(b.Repo.Name); r.Branch == "" {
				r.Branch = key
			}
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
	Key      string      `json:"key"`
	Title    string      `json:"title"`
	URL      string      `json:"url"`
	Repos    []StartRepo `json:"repos"`
	SetAside bool        `json:"setAside"`
	Checks   []string    `json:"checks"`
}

type StartRepo struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
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
	for i, sr := range req.Repos {
		targets[i] = testrun.Target{Name: sr.Name, Branch: sr.Branch}
		if r, err := workspace.Resolve(a.roots, sr.Name); err == nil {
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
		for _, c := range req.Checks {
			s.Checks = append(s.Checks, testrun.Check{Text: c})
		}
		if st.Current != nil {
			s.Started, s.Checks = st.Current.Started, st.Current.Checks
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

// TesterChecks saves the checklist of the change under test.
func (a *App) TesterChecks(checks []testrun.Check) error {
	a.testMu.Lock()
	defer a.testMu.Unlock()
	st, err := a.testStore().Load()
	if err != nil {
		return err
	}
	if st.Current == nil {
		return errors.New("nothing is being tested")
	}
	st.Current.Checks = checks
	return a.testStore().Save(st)
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

// VerdictOption is one of the ticket's transitions offered as a test result.
type VerdictOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	To      string `json:"to"`
	Outcome string `json:"outcome"`
}

// failWords mark a transition that sends a ticket back, whatever its target's
// category says.
var failWords = []string{"fail", "reject", "require change", "reopen", "back to"}

func outcomeOf(t jira.Transition) string {
	text := strings.ToLower(t.Name + " " + t.To)
	for _, w := range failWords {
		if strings.Contains(text, w) {
			return "failed"
		}
	}
	if t.ToCategory == "done" {
		return "passed"
	}
	return "moved"
}

func (a *App) ticketRef(key, url string) (jira.Ref, error) {
	if r, ok := jira.Parse(url); ok {
		return r, nil
	}
	if site := a.Settings().Jira.Site; site != "" {
		return jira.Ref{Site: site, Key: key}, nil
	}
	return jira.Ref{}, errors.New("add your Jira site in Settings to record results")
}

// TesterVerdicts lists the moves key's workflow allows from its status now,
// passes first, as the results a tester can record.
func (a *App) TesterVerdicts(key string) ([]VerdictOption, error) {
	ref, err := a.ticketRef(key, "")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	ts, err := a.jiraClient().Transitions(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := make([]VerdictOption, len(ts))
	for i, t := range ts {
		out[i] = VerdictOption{ID: t.ID, Name: t.Name, To: t.To, Outcome: outcomeOf(t)}
	}
	rank := map[string]int{"passed": 0, "failed": 1, "moved": 2}
	slices.SortStableFunc(out, func(x, y VerdictOption) int { return rank[x.Outcome] - rank[y.Outcome] })
	return out, nil
}

// Attachment is a file to add to the ticket, its content base64-encoded.
type Attachment struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

// VerdictRequest is a test result: the transition to make, a note, whether to
// add the checklist to the comment, and files such as screenshots.
type VerdictRequest struct {
	ID         string       `json:"id"`
	Note       string       `json:"note"`
	WithChecks bool         `json:"withChecks"`
	Files      []Attachment `json:"files"`
}

// verdictComment is the comment a verdict adds: the note, the checklist and
// the files attached, or "" when there's none of those.
func verdictComment(note string, checks []testrun.Check, files []string) string {
	var parts []string
	if n := strings.TrimSpace(note); n != "" {
		parts = append(parts, n)
	}
	if len(checks) > 0 {
		lines := []string{"Checked:"}
		for _, c := range checks {
			mark := "[ ]"
			if c.Done {
				mark = "[x]"
			}
			lines = append(lines, mark+" "+c.Text)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if len(files) > 0 {
		parts = append(parts, "Attached: "+strings.Join(files, ", "))
	}
	return strings.Join(parts, "\n\n")
}

// TesterVerdict moves the ticket under test along the chosen transition,
// attaches the files, comments, and records the verdict for the history.
func (a *App) TesterVerdict(req VerdictRequest) (string, error) {
	id, note := req.ID, req.Note
	a.testMu.Lock()
	defer a.testMu.Unlock()
	st, err := a.testStore().Load()
	if err != nil {
		return "", err
	}
	s := st.Current
	if s == nil {
		return "", errors.New("nothing is being tested")
	}
	ref, err := a.ticketRef(s.Key, s.URL)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	c := a.jiraClient()
	ts, err := c.Transitions(ctx, ref)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(ts, func(t jira.Transition) bool { return t.ID == id })
	if i < 0 {
		return "", fmt.Errorf("%s can't make that move any more: its status may have changed", s.Key)
	}
	t := ts[i]
	files := make([][]byte, len(req.Files))
	for i, f := range req.Files {
		if files[i], err = base64.StdEncoding.DecodeString(f.Data); err != nil {
			return "", fmt.Errorf("reading %s: %w", f.Name, err)
		}
	}
	if err := c.Transition(ctx, ref, id); err != nil {
		return "", err
	}
	s.Verdict = &testrun.Verdict{Name: t.Name, To: t.To, Outcome: outcomeOf(t), Note: strings.TrimSpace(note)}
	if err := a.testStore().Save(st); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.queue = nil
	a.mu.Unlock()
	msg := s.Key + " moved to " + t.To
	var attached, problems []string
	for i, f := range req.Files {
		if err := c.Attach(ctx, ref, f.Name, files[i]); err != nil {
			problems = append(problems, f.Name+" wasn't attached: "+err.Error())
		} else {
			attached = append(attached, f.Name)
		}
	}
	var checks []testrun.Check
	if req.WithChecks {
		checks = s.Checks
	}
	if text := verdictComment(note, checks, attached); text != "" {
		if err := c.Comment(ctx, ref, text); err != nil {
			problems = append(problems, "the comment wasn't added: "+err.Error())
		} else {
			msg += ", with a comment"
		}
	}
	if len(problems) > 0 {
		msg += ", but " + strings.Join(problems, "; ")
	}
	return msg, nil
}

type TestFinish struct {
	Steps []testrun.Step `json:"steps"`
	Done  bool           `json:"done"`
}

// TesterFinish puts every repo under test back on its base. Repos that can't
// go back stay in the session for another try.
func (a *App) TesterFinish() (TestFinish, error) {
	a.testMu.Lock()
	defer a.testMu.Unlock()
	st, err := a.testStore().Load()
	if err != nil {
		return TestFinish{}, err
	}
	if st.Current == nil {
		return TestFinish{}, errors.New("nothing is being tested")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	steps, left := testrun.End(ctx, st.Current)
	out := TestFinish{Steps: steps, Done: len(left) == 0}
	if out.Done {
		st.History = append([]testrun.Record{st.Current.Record()}, st.History...)
		st.Current = nil
	} else {
		st.Current.Repos = left
	}
	if err := a.testStore().Save(st); err != nil {
		return out, err
	}
	a.mu.Lock()
	a.queue = nil
	a.mu.Unlock()
	return out, nil
}

var (
	acceptanceHeading = regexp.MustCompile(`(?i)^(acceptance criteria|acceptance|to test|test plan|testing|how to test|steps to test)\b`)
	listItem          = regexp.MustCompile(`^(?:• |[0-9]+\. |- \[[ xX]\] |[-*] )(.+)`)
)

// checklist picks what to try out of a ticket's description: the list under an
// acceptance-criteria or testing heading, or failing that every top-level list
// item. Nested items belong to their parent.
func checklist(desc string) []string {
	var all, under []string
	in, found := false, false
	for _, line := range strings.Split(desc, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := listItem.FindStringSubmatch(line)
		switch {
		case m != nil:
			item := strings.TrimSpace(m[1])
			all = append(all, item)
			if in {
				under = append(under, item)
			}
		case strings.HasPrefix(line, " "):
		case acceptanceHeading.MatchString(strings.TrimSpace(line)):
			in, found = true, true
		default:
			in = false
		}
	}
	out := all
	if found {
		out = under
	}
	if len(out) > 30 {
		out = out[:30]
	}
	return append([]string{}, out...)
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/checks"
	"github.com/jonezzyboy/tandem/internal/core"
	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/workspace"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	localEvery  = 4 * time.Second
	remoteEvery = 60 * time.Second
	allEvery    = 5 * time.Minute
	// A focused change older than this gets a GitHub refresh straight away.
	remoteStale = 20 * time.Second
	inboxStale  = 60 * time.Second
	viewFile    = ".view.json"
)

// App is the API bound to the frontend. Views are served from memory (seeded
// from disk at startup) so screens render immediately; refreshes run in the
// background and push "change" events only when something differs.
type App struct {
	ctx   context.Context
	store change.Store
	roots []string
	emit  func(event string, data any)

	mu       sync.Mutex
	views    map[string]core.ChangeView
	remoteAt map[string]time.Time
	focus    string
	inflight map[string]bool
	ops      map[string]*sync.Mutex
	inbox    *Inbox
	inboxAt  time.Time
	trains   map[string]context.CancelFunc
	settings Settings
	account  *Account
}

func NewApp(store change.Store, roots []string) *App {
	return &App{
		store:    store,
		roots:    roots,
		emit:     func(string, any) {},
		views:    map[string]core.ChangeView{},
		remoteAt: map[string]time.Time{},
		inflight: map[string]bool{},
		ops:      map[string]*sync.Mutex{},
		trains:   map[string]context.CancelFunc{},
		settings: defaultSettings(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emit = func(event string, data any) { runtime.EventsEmit(ctx, event, data) }
	a.loadCached()
	go a.poll(ctx)
}

func (a *App) loadCached() {
	all, _ := a.store.List()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range all {
		data, err := os.ReadFile(filepath.Join(a.store.Dir(c.ID), viewFile))
		if err != nil {
			continue
		}
		var v core.ChangeView
		if json.Unmarshal(data, &v) == nil {
			a.views[c.ID] = v
		}
	}
}

func (a *App) poll(ctx context.Context) {
	go a.refreshAll()
	local := time.NewTicker(localEvery)
	remote := time.NewTicker(remoteEvery)
	all := time.NewTicker(allEvery)
	defer local.Stop()
	defer remote.Stop()
	defer all.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-local.C:
			if id := a.focused(); id != "" {
				go a.refresh(id, false)
			}
		case <-remote.C:
			if id := a.focused(); id != "" {
				go a.refresh(id, true)
			}
		case <-all.C:
			go a.refreshAll()
		}
	}
}

func (a *App) refreshAll() {
	all, err := a.store.List()
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, c := range all {
		wg.Go(func() { a.refresh(c.ID, true) })
	}
	wg.Go(func() { a.fetchInbox() })
	wg.Wait()
}

func (a *App) focused() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.focus
}

// begin claims key so the same refresh never runs twice at once.
func (a *App) begin(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inflight[key] {
		return false
	}
	a.inflight[key] = true
	return true
}

func (a *App) end(key string) {
	a.mu.Lock()
	delete(a.inflight, key)
	a.mu.Unlock()
}

func (a *App) opLock(id string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ops[id] == nil {
		a.ops[id] = &sync.Mutex{}
	}
	return a.ops[id]
}

func (a *App) refresh(id string, remote bool) {
	key := id + ":local"
	if remote {
		key = id + ":remote"
	}
	if !a.begin(key) {
		return
	}
	defer a.end(key)
	if _, err := a.buildView(id, remote); err != nil {
		a.emit("error", err.Error())
	}
}

func (a *App) buildView(id string, remote bool) (core.ChangeView, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return core.ChangeView{}, err
	}
	g, graphErr := core.BuildGraph(c)
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	defer cancel()
	states := core.Snapshot(ctx, c, remote)
	if remote {
		if err := a.store.Save(c); err != nil {
			return core.ChangeView{}, err
		}
	}
	v := core.BuildView(c, g, graphErr, states, remote)

	a.mu.Lock()
	prev, had := a.views[id]
	if !remote && had {
		v = core.MergeLocal(prev, v)
	}
	changed := !had || !sameView(prev, v)
	a.views[id] = v
	if remote {
		a.remoteAt[id] = time.Now()
	}
	a.mu.Unlock()

	if changed {
		a.persist(v)
		a.emit("change", v)
		a.emit("changes", a.Changes())
	}
	return v, nil
}

func sameView(x, y core.ChangeView) bool {
	x.CheckedAt, y.CheckedAt = time.Time{}, time.Time{}
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return string(bx) == string(by)
}

func (a *App) persist(v core.ChangeView) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	path := filepath.Join(a.store.Dir(v.ID), viewFile)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, path)
	}
}

type ChangeSummary struct {
	ID string `json:"id"`
	// CheckedOut means every leg's repo has the change's branch checked out.
	CheckedOut bool      `json:"checkedOut"`
	Title      string    `json:"title"`
	Legs       int       `json:"legs"`
	Blocked    int       `json:"blocked"`
	Failing    int       `json:"failing"`
	Remote     bool      `json:"remote"`
	Headline   string    `json:"headline"`
	Tone       string    `json:"tone"`
	Created    time.Time `json:"created"`
}

// Changes lists every change, newest first, summarised from the cached views.
func (a *App) Changes() []ChangeSummary {
	all, _ := a.store.List()
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ChangeSummary, 0, len(all))
	for _, c := range all {
		s := ChangeSummary{ID: c.ID, Title: c.Title, Legs: len(c.Legs), Created: c.Created, Tone: "muted"}
		legs := plural(s.Legs, "leg")
		v, ok := a.views[c.ID]
		if ok && len(v.Legs) > 0 {
			s.CheckedOut = true
			for _, l := range v.Legs {
				s.CheckedOut = s.CheckedOut && l.OnBranch
			}
		}
		switch {
		case !ok:
			s.Headline = legs + " · not checked yet"
		case !v.Remote:
			s.Headline = legs + " · local only"
		default:
			s.Remote, s.Blocked = true, v.Blocked
			merged := 0
			for _, l := range v.Legs {
				if l.PR != nil && l.PR.Fail > 0 {
					s.Failing++
				}
				if l.PR != nil && l.PR.State == "MERGED" {
					merged++
				}
			}
			switch {
			case merged > 0 && merged == len(v.Legs):
				s.Headline, s.Tone = legs+" · merged", "ok"
			case s.Failing > 0:
				s.Headline, s.Tone = fmt.Sprintf("%s · %d failing", legs, s.Failing), "warn"
			case s.Blocked > 0:
				s.Headline = fmt.Sprintf("%s · %d blocked", legs, s.Blocked)
			default:
				s.Headline, s.Tone = legs+" · ready to merge", "ok"
			}
		}
		out = append(out, s)
	}
	return out
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// Change returns the cached view at once when there is one, and refreshes in
// the background; otherwise it builds a local view first.
func (a *App) Change(id string) (core.ChangeView, error) {
	a.mu.Lock()
	v, ok := a.views[id]
	a.mu.Unlock()
	if ok {
		go a.refresh(id, false)
		return v, nil
	}
	v, err := a.buildView(id, false)
	if err != nil {
		return v, err
	}
	go a.refresh(id, true)
	return v, nil
}

// Focus tells the poller which change is on screen. Called on navigation and
// whenever the window regains focus.
func (a *App) Focus(id string) {
	a.mu.Lock()
	a.focus = id
	stale := time.Since(a.remoteAt[id]) > remoteStale
	a.mu.Unlock()
	if id == "" {
		return
	}
	go a.refresh(id, false)
	if stale {
		go a.refresh(id, true)
	}
}

func (a *App) Refresh(id string) {
	go a.refresh(id, true)
}

type LegResult struct {
	Leg     string `json:"leg"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func (a *App) Sync(id string) ([]LegResult, error) {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	g, _ := core.BuildGraph(c)
	var out []LegResult
	for _, r := range core.Sync(a.ctx, c, core.OrderedLegs(c, g.Levels)) {
		out = append(out, LegResult{Leg: r.Leg.Name(), OK: r.OK, Message: r.Message})
	}
	go a.refresh(id, false)
	return out, nil
}

type CheckEvent struct {
	Change string `json:"change"`
	Leg    string `json:"leg"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Millis int64  `json:"ms"`
	Output string `json:"output"`
}

// Check runs local checks for one leg, or every leg when leg is empty, and
// streams each check's progress as "check" events.
func (a *App) Check(id, leg string) ([]CheckEvent, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	var legs []*change.Leg
	if leg == "" {
		g, _ := core.BuildGraph(c)
		legs = core.OrderedLegs(c, g.Levels)
	} else {
		l, err := c.Leg(leg)
		if err != nil {
			return nil, err
		}
		legs = []*change.Leg{l}
	}
	var mu sync.Mutex
	var out []CheckEvent
	var errs []error
	var wg sync.WaitGroup
	for _, l := range legs {
		wg.Go(func() {
			_, err := core.RunChecks(a.ctx, c, l, func(r checks.Result, done bool) {
				ev := checkEvent(id, l.Name(), r, done)
				a.emit("check", ev)
				if done {
					mu.Lock()
					out = append(out, ev)
					mu.Unlock()
				}
			})
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", l.Name(), err))
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return out, errors.Join(errs...)
}

func checkEvent(id, leg string, r checks.Result, done bool) CheckEvent {
	ev := CheckEvent{Change: id, Leg: leg, Name: r.Check.Name, State: "running"}
	switch {
	case !done:
	case r.Check.Skip != "":
		ev.State, ev.Output = "skip", r.Check.Skip
	case r.Err != nil:
		ev.State, ev.Output = "fail", r.Output
	default:
		ev.State = "pass"
	}
	ev.Millis = r.Duration.Milliseconds()
	return ev
}

type PRRequest struct {
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	Reviewers      []string `json:"reviewers"`
	Draft          bool     `json:"draft"`
	ForceWithLease bool     `json:"forceWithLease"`
}

type PlanItem struct {
	Repo   string `json:"repo"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Action string `json:"action"`
	Note   string `json:"note"`
	Error  string `json:"error"`
	Dirty  int    `json:"dirty"`
	URL    string `json:"url"`
}

type PRPreview struct {
	Title string     `json:"title"`
	Items []PlanItem `json:"items"`
}

func (a *App) prepare(id string, req PRRequest) (*change.Change, string, core.Graph, error) {
	c, err := a.store.Load(id)
	if err != nil {
		return nil, "", core.Graph{}, err
	}
	if t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.Title), c.ID)); t != "" {
		c.Title = t
	}
	c.Body = req.Body
	c.Reviewers = req.Reviewers
	title, err := core.PRTitle(c, "")
	if err != nil {
		return nil, "", core.Graph{}, err
	}
	g, err := core.BuildGraph(c)
	return c, title, g, err
}

// PlanPRs previews what PublishPRs would do; nothing is pushed or saved.
func (a *App) PlanPRs(id string, req PRRequest) (PRPreview, error) {
	c, title, g, err := a.prepare(id, req)
	if err != nil {
		return PRPreview{}, err
	}
	return preview(title, core.PlanPRs(a.ctx, c, g, req.Draft)), nil
}

func (a *App) PublishPRs(id string, req PRRequest) (PRPreview, error) {
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	c, title, g, err := a.prepare(id, req)
	if err != nil {
		return PRPreview{}, err
	}
	plans := core.PlanPRs(a.ctx, c, g, req.Draft)
	core.PublishPRs(a.ctx, c, plans, title, core.PROptions{Draft: req.Draft, ForceWithLease: req.ForceWithLease})
	if err := a.store.Save(c); err != nil {
		return PRPreview{}, err
	}
	go a.refresh(id, true)
	return preview(title, plans), nil
}

func preview(title string, plans []*core.PRPlan) PRPreview {
	p := PRPreview{Title: title, Items: []PlanItem{}}
	for _, pl := range plans {
		it := PlanItem{Repo: pl.Leg.Repo, Name: pl.Leg.Name(), Level: pl.Level, Note: pl.Note, Dirty: pl.State.Status.Dirty}
		switch pl.Action {
		case core.PRCreate:
			it.Action = "create"
		case core.PRUpdate:
			it.Action = "update"
		default:
			it.Action = "skip"
		}
		if pl.Err != nil {
			it.Error = core.FirstLine(pl.Err.Error())
		}
		if pl.State.PR != nil {
			it.URL = pl.State.PR.URL
		}
		p.Items = append(p.Items, it)
	}
	return p
}

type InboxItem struct {
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Author    string    `json:"author"`
	Draft     bool      `json:"draft"`
	UpdatedAt time.Time `json:"updatedAt"`
	ChangeID  string    `json:"changeId"`
}

type Inbox struct {
	Review      []InboxItem `json:"review"`
	Mine        []InboxItem `json:"mine"`
	ReviewError string      `json:"reviewError"`
	MineError   string      `json:"mineError"`
	FetchedAt   time.Time   `json:"fetchedAt"`
}

// Inbox returns review requests (longest waiting first) and your open PRs,
// each tagged with the change it belongs to when Tandem tracks it.
func (a *App) Inbox(force bool) Inbox {
	a.mu.Lock()
	cached, at := a.inbox, a.inboxAt
	a.mu.Unlock()
	if cached != nil && !force && time.Since(at) < inboxStale {
		return *cached
	}
	return a.fetchInbox()
}

func (a *App) fetchInbox() Inbox {
	owners := map[string]string{}
	if all, err := a.store.List(); err == nil {
		for _, c := range all {
			for _, l := range c.Legs {
				if l.PRURL != "" {
					owners[l.PRURL] = c.ID
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	var in Inbox
	var review, mine []gh.SearchPR
	var reviewErr, mineErr error
	var wg sync.WaitGroup
	wg.Go(func() { review, reviewErr = gh.Search(ctx, "--review-requested=@me", "--state=open") })
	wg.Go(func() { mine, mineErr = gh.Search(ctx, "--author=@me", "--state=open") })
	wg.Wait()
	convert := func(prs []gh.SearchPR) []InboxItem {
		out := make([]InboxItem, 0, len(prs))
		for _, p := range prs {
			out = append(out, InboxItem{
				Repo: p.Repository.NameWithOwner, Number: p.Number, Title: p.Title, URL: p.URL,
				Author: p.Author.Login, Draft: p.IsDraft, UpdatedAt: p.UpdatedAt, ChangeID: owners[p.URL],
			})
		}
		return out
	}
	in.Review, in.Mine = convert(review), convert(mine)
	sort.Slice(in.Review, func(i, j int) bool { return in.Review[i].UpdatedAt.Before(in.Review[j].UpdatedAt) })
	sort.Slice(in.Mine, func(i, j int) bool { return in.Mine[i].UpdatedAt.After(in.Mine[j].UpdatedAt) })
	if reviewErr != nil {
		in.ReviewError = core.FirstLine(reviewErr.Error())
	}
	if mineErr != nil {
		in.MineError = core.FirstLine(mineErr.Error())
	}
	in.FetchedAt = time.Now()
	a.mu.Lock()
	a.inbox, a.inboxAt = &in, in.FetchedAt
	a.mu.Unlock()
	a.emit("inbox", in)
	return in
}

type RepoInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (a *App) Repos() []RepoInfo {
	repos := workspace.List(a.roots)
	out := make([]RepoInfo, len(repos))
	for i, r := range repos {
		out[i] = RepoInfo{Name: r.Name, Path: r.Path}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type StartRequest struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Repos []string `json:"repos"`
}

type StartItem struct {
	Repo     string `json:"repo"`
	OK       bool   `json:"ok"`
	Existing bool   `json:"existing"`
	Message  string `json:"message"`
}

func (a *App) Start(req StartRequest) ([]StartItem, error) {
	id := strings.TrimSpace(req.ID)
	lock := a.opLock(id)
	lock.Lock()
	defer lock.Unlock()
	if len(req.Repos) == 0 {
		return nil, errors.New("pick at least one repo")
	}
	var repos []workspace.Repo
	for _, name := range req.Repos {
		r, err := workspace.Resolve(a.roots, name)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	_, results, err := core.Start(a.ctx, a.store, id, strings.TrimSpace(req.Title), repos)
	if err != nil {
		return nil, err
	}
	out := make([]StartItem, len(results))
	for i, r := range results {
		it := StartItem{Repo: r.Repo.Name, OK: r.Err == nil, Existing: r.Existing}
		switch {
		case r.Existing:
			it.Message = "already in " + id
		case r.Err != nil:
			it.Message = core.FirstLine(r.Err.Error())
		default:
			it.Message = "branch " + id
			if !r.Created {
				it.Message += " (existing)"
			}
			if r.Switched {
				it.Message += ", checked out"
			}
			if r.Warning != "" {
				it.Message += " · " + r.Warning
			}
		}
		out[i] = it
	}
	a.emit("changes", a.Changes())
	go a.refresh(id, true)
	return out, nil
}

func (a *App) Link(id, upstream, downstream string) error {
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
	c.AddDeclared(change.Edge{From: up.Repo, To: down.Repo})
	if _, err := core.BuildGraph(c); err != nil {
		return err
	}
	if err := a.store.Save(c); err != nil {
		return err
	}
	go a.refresh(id, false)
	return nil
}

func (a *App) OpenURL(url string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing to open %q", url)
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}

// within keeps the frontend's open-folder/editor calls to a leg's repo or
// Tandem's home.
func (a *App) within(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	allowed := []string{a.store.Home}
	if all, err := a.store.List(); err == nil {
		for _, c := range all {
			for _, l := range c.Legs {
				allowed = append(allowed, l.Dir())
			}
		}
	}
	for _, dir := range allowed {
		rel, err := filepath.Rel(dir, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("%s is not a repo in any change", path)
}

func (a *App) OpenFolder(path string) error {
	p, err := a.within(path)
	if err != nil {
		return err
	}
	return exec.Command("open", p).Start()
}

// OpenEditor opens path with the editor from Settings.
func (a *App) OpenEditor(path string) error {
	p, err := a.within(path)
	if err != nil {
		return err
	}
	cmd, err := a.editorCommand()
	if err != nil {
		return err
	}
	return exec.Command(cmd[0], append(cmd[1:], p)...).Start()
}

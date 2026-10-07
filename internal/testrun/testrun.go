// Package testrun puts every repo of a change on its branch for testing, and
// back on its base afterwards, setting aside and restoring uncommitted work.
package testrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jonezzyboy/tandem/internal/gh"
	"github.com/jonezzyboy/tandem/internal/gitx"
)

// Session is a change being tested: the repos switched to its branch, and
// what to put back when testing ends.
type Session struct {
	Key     string    `json:"key"`
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Started time.Time `json:"started"`
	Repos   []Repo    `json:"repos"`
	// Verdict is the result recorded on the ticket, before the repos go back.
	Verdict *Verdict `json:"verdict,omitempty"`
}

// Verdict is the move a tester made on the ticket. Outcome is "passed",
// "failed" or "moved" (neither, such as on to another stage).
type Verdict struct {
	Name    string `json:"name"`
	To      string `json:"to"`
	Outcome string `json:"outcome"`
}

type Repo struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
	// Branch is the change's branch in this repo: the ticket key, or a name
	// starting with it such as DEV-12-fix-prices.
	Branch string `json:"branch,omitempty"`
	// Stash is the commit of the stash holding work set aside to switch.
	Stash string `json:"stash,omitempty"`
}

// Record is a finished test, newest first in State.History. Result is the
// verdict's outcome, or "stopped" when there was none.
type Record struct {
	Key     string    `json:"key"`
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Result  string    `json:"result"`
	Verdict string    `json:"verdict,omitempty"`
	To      string    `json:"to,omitempty"`
	At      time.Time `json:"at"`
}

// Record is the history entry for s, finished now.
func (s *Session) Record() Record {
	r := Record{Key: s.Key, Title: s.Title, URL: s.URL, Result: "stopped", At: time.Now().UTC()}
	if s.Verdict != nil {
		r.Result, r.Verdict, r.To = s.Verdict.Outcome, s.Verdict.Name, s.Verdict.To
	}
	return r
}

type State struct {
	Current *Session `json:"current,omitempty"`
	History []Record `json:"history"`
}

const historyLimit = 50

// Store keeps State in one JSON file.
type Store struct {
	Path string
}

func (s Store) Load() (State, error) {
	var st State
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("read %s: %w", s.Path, err)
	}
	return st, nil
}

func (s Store) Save(st State) error {
	if len(st.History) > historyLimit {
		st.History = st.History[:historyLimit]
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Target is a repo with the change's branch; Dir is empty when it isn't cloned,
// and Branch empty means the branch is named after the key.
type Target struct {
	Name   string
	Dir    string
	Branch string
}

// branch is r's branch for the change keyed key, for sessions saved before
// repos recorded their own.
func (r Repo) branch(key string) string {
	if r.Branch != "" {
		return r.Branch
	}
	return key
}

// Step is one repo's progress or outcome; Done is false while it's under way.
type Step struct {
	Repo    string `json:"repo"`
	Done    bool   `json:"done"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	SHA     string `json:"sha"`
}

type Options struct {
	// SetAside stashes uncommitted work instead of refusing to switch.
	SetAside bool
	// CloneRoot holds <owner>/<repo> clones made for repos not yet cloned; empty
	// refuses those repos.
	CloneRoot string
	Clone     func(ctx context.Context, repo, dir string) error
	OnStep    func(Step)
}

// Begin switches every target to branch key, concurrently. The session holds
// the repos that switched; Steps say what happened to each.
func Begin(ctx context.Context, key, title, url string, targets []Target, o Options) (*Session, []Step) {
	if o.Clone == nil {
		o.Clone = gh.Clone
	}
	report := reporter(o.OnStep)
	steps := make([]Step, len(targets))
	repos := make([]*Repo, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() {
			repos[i], steps[i] = begin(ctx, key, t, o, report)
			report(steps[i])
		})
	}
	wg.Wait()
	s := &Session{Key: key, Title: title, URL: url, Started: time.Now().UTC()}
	for _, r := range repos {
		if r != nil {
			s.Repos = append(s.Repos, *r)
		}
	}
	return s, steps
}

func begin(ctx context.Context, key string, t Target, o Options, report func(Step)) (*Repo, Step) {
	step := Step{Repo: t.Name, Done: true}
	fail := func(msg string) (*Repo, Step) {
		step.Message = msg
		return nil, step
	}
	r := Repo{Name: t.Name, Dir: t.Dir, Branch: t.Branch}
	branch := r.branch(key)
	var notes []string
	if r.Dir == "" {
		if o.CloneRoot == "" {
			return fail("not cloned on this machine")
		}
		r.Dir = filepath.Join(o.CloneRoot, filepath.FromSlash(t.Name))
		report(Step{Repo: t.Name, Message: "cloning into " + r.Dir + "…"})
		if err := o.Clone(ctx, t.Name, r.Dir); err != nil {
			return fail("cloning: " + firstLine(err))
		}
		notes = append(notes, "cloned")
	} else {
		report(Step{Repo: t.Name, Message: "fetching…"})
		if err := gitx.Fetch(ctx, r.Dir); err != nil {
			return fail("fetching: " + firstLine(err))
		}
		notes = append(notes, "fetched")
	}
	if dirty := uncommitted(ctx, r.Dir); dirty > 0 && gitx.CurrentBranch(ctx, r.Dir) != branch {
		if !o.SetAside {
			return fail(fmt.Sprintf("%d uncommitted files: commit or set them aside first", dirty))
		}
		sha, err := stash(ctx, r.Dir, "tandem: set aside to test "+key)
		if err != nil {
			return fail("setting aside uncommitted work: " + firstLine(err))
		}
		r.Stash = sha
		notes = append(notes, fmt.Sprintf("set aside %d uncommitted files", dirty))
	}
	warning, err := switchTo(ctx, r.Dir, branch)
	if err != nil {
		if r.Stash != "" {
			restore(ctx, r.Dir, r.Stash)
		}
		return fail(firstLine(err))
	}
	notes = append(notes, "switched to "+branch)
	if warning != "" {
		notes = append(notes, warning)
	}
	step.OK, step.Message, step.SHA = true, strings.Join(notes, " · "), short(ctx, r.Dir, "HEAD")
	return &r, step
}

// switchTo checks out branch at origin's latest. A local branch with commits
// origin lacks is left as it is, with a warning.
func switchTo(ctx context.Context, dir, branch string) (string, error) {
	remote := gitx.RefExists(ctx, dir, "refs/remotes/origin/"+branch)
	if !gitx.RefExists(ctx, dir, "refs/heads/"+branch) {
		if !remote {
			return "", fmt.Errorf("no %s branch here or on origin", branch)
		}
		_, err := gitx.Run(ctx, dir, "switch", "--quiet", "-c", branch, "--track", "origin/"+branch)
		return "", err
	}
	if _, err := gitx.Run(ctx, dir, "switch", "--quiet", branch); err != nil {
		return "", err
	}
	if !remote {
		return "", nil
	}
	if _, err := gitx.Run(ctx, dir, "merge", "--ff-only", "--quiet", "origin/"+branch); err != nil {
		return "local " + branch + " has commits origin doesn't, so it was left as it is", nil
	}
	return "", nil
}

// RepoStatus is where a session's repo stands against origin.
type RepoStatus struct {
	Name     string `json:"name"`
	Branch   string `json:"branch"`
	OnBranch bool   `json:"onBranch"`
	Current  string `json:"current"`
	// Behind counts commits pushed to the branch since the checkout; New holds
	// the newest of their subjects.
	Behind int      `json:"behind"`
	New    []string `json:"new"`
	Dirty  int      `json:"dirty"`
	Error  string   `json:"error"`
}

// Status fetches every repo of s and compares it with origin's branch.
func Status(ctx context.Context, s *Session) []RepoStatus {
	out := make([]RepoStatus, len(s.Repos))
	var wg sync.WaitGroup
	for i, r := range s.Repos {
		wg.Go(func() {
			st := RepoStatus{Name: r.Name, New: []string{}}
			if err := gitx.Fetch(ctx, r.Dir); err != nil {
				st.Error = "fetching: " + firstLine(err)
			}
			branch := r.branch(s.Key)
			st.Branch = branch
			st.Current = gitx.CurrentBranch(ctx, r.Dir)
			st.OnBranch = st.Current == branch
			st.Dirty = uncommitted(ctx, r.Dir)
			if st.OnBranch && gitx.RefExists(ctx, r.Dir, "refs/remotes/origin/"+branch) {
				rng := "HEAD..origin/" + branch
				if n, err := gitx.Run(ctx, r.Dir, "rev-list", "--count", rng); err == nil {
					st.Behind, _ = strconv.Atoi(n)
				}
				if st.Behind > 0 {
					if log, err := gitx.Run(ctx, r.Dir, "log", "--format=%s", "-n", "5", rng); err == nil && log != "" {
						st.New = strings.Split(log, "\n")
					}
				}
			}
			out[i] = st
		})
	}
	wg.Wait()
	return out
}

// Pull fast-forwards every repo of s that is on the branch to origin's latest.
func Pull(ctx context.Context, s *Session) []Step {
	out := make([]Step, len(s.Repos))
	var wg sync.WaitGroup
	for i, r := range s.Repos {
		wg.Go(func() {
			st := Step{Repo: r.Name, Done: true}
			branch := r.branch(s.Key)
			switch {
			case gitx.CurrentBranch(ctx, r.Dir) != branch:
				st.Message = "not on " + branch + ", left alone"
			case gitx.Fetch(ctx, r.Dir) != nil:
				st.Message = "couldn't fetch"
			default:
				before := short(ctx, r.Dir, "HEAD")
				if _, err := gitx.Run(ctx, r.Dir, "merge", "--ff-only", "--quiet", "origin/"+branch); err != nil {
					st.Message = firstLine(err)
				} else {
					st.OK, st.SHA = true, short(ctx, r.Dir, "HEAD")
					st.Message = "up to date"
					if st.SHA != before {
						st.Message = "pulled to " + st.SHA
					}
				}
			}
			out[i] = st
		})
	}
	wg.Wait()
	return out
}

// End puts every repo of s back on its base at origin's latest and restores
// work set aside. It returns the repos that couldn't go back, so a retry can
// pick them up.
func End(ctx context.Context, s *Session) ([]Step, []Repo) {
	steps := make([]Step, len(s.Repos))
	var wg sync.WaitGroup
	for i, r := range s.Repos {
		wg.Go(func() { steps[i] = end(ctx, r) })
	}
	wg.Wait()
	var left []Repo
	for i, st := range steps {
		if !st.OK {
			left = append(left, s.Repos[i])
		}
	}
	return steps, left
}

func end(ctx context.Context, r Repo) Step {
	st := Step{Repo: r.Name, Done: true}
	if dirty := uncommitted(ctx, r.Dir); dirty > 0 {
		st.Message = fmt.Sprintf("%d uncommitted files from testing: commit or discard them, then try again", dirty)
		return st
	}
	gitx.Fetch(ctx, r.Dir)
	base, err := gitx.DefaultBase(ctx, r.Dir)
	if err != nil {
		st.Message = firstLine(err)
		return st
	}
	if err := gitx.Switch(ctx, r.Dir, base.Branch); err != nil {
		st.Message = firstLine(err)
		return st
	}
	notes := []string{"on " + base.Branch}
	if base.Ref != base.Branch {
		before := short(ctx, r.Dir, "HEAD")
		if _, err := gitx.Run(ctx, r.Dir, "merge", "--ff-only", "--quiet", base.Ref); err != nil {
			notes = append(notes, "local "+base.Branch+" has its own commits, so it wasn't updated")
		} else if n, err := gitx.Run(ctx, r.Dir, "rev-list", "--count", before+"..HEAD"); err == nil && n != "0" {
			notes = append(notes, "pulled "+n+" new commits")
		}
	}
	if r.Stash != "" {
		if err := restore(ctx, r.Dir, r.Stash); err != nil {
			notes = append(notes, "your set-aside work is still stashed: "+firstLine(err))
		} else {
			notes = append(notes, "your uncommitted work is back")
		}
	}
	st.OK, st.Message, st.SHA = true, strings.Join(notes, " · "), short(ctx, r.Dir, "HEAD")
	return st
}

func uncommitted(ctx context.Context, dir string) int {
	out, err := gitx.Run(ctx, dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil || out == "" {
		return 0
	}
	return strings.Count(out, "\n") + 1
}

func stash(ctx context.Context, dir, message string) (string, error) {
	if _, err := gitx.Run(ctx, dir, "stash", "push", "--quiet", "-m", message); err != nil {
		return "", err
	}
	return gitx.Run(ctx, dir, "rev-parse", "refs/stash")
}

// restore pops the stash whose commit is sha, wherever later stashes have
// pushed it in the list.
func restore(ctx context.Context, dir, sha string) error {
	list, err := gitx.Run(ctx, dir, "stash", "list", "--format=%H")
	if err != nil {
		return err
	}
	for i, h := range strings.Split(list, "\n") {
		if h == sha {
			_, err := gitx.Run(ctx, dir, "stash", "pop", "--quiet", "stash@{"+strconv.Itoa(i)+"}")
			return err
		}
	}
	return errors.New("the stash is gone")
}

func short(ctx context.Context, dir, ref string) string {
	out, _ := gitx.Run(ctx, dir, "rev-parse", "--short", ref)
	return out
}

func firstLine(err error) string {
	s, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
	return s
}

// reporter serialises a possibly nil callback for concurrent callers.
func reporter(fn func(Step)) func(Step) {
	var mu sync.Mutex
	return func(s Step) {
		if fn == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		fn(s)
	}
}

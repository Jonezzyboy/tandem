// Package change holds the Change model: one unit of work spanning several
// repos, each repo's part being a Leg.
package change

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

type Leg struct {
	Repo   string `json:"repo"`
	Source string `json:"source"`
	// Worktree is set for legs of a change made with worktrees (and legs from
	// before branches were the default); Dir is where a leg's files are either way.
	Worktree string `json:"worktree,omitempty"`
	Base     string `json:"base"`
	BaseRef  string `json:"baseRef"`
	PR       int    `json:"pr,omitempty"`
	PRURL    string `json:"prUrl,omitempty"`
}

// Name is the repo name without its vendor, used in CLI args.
func (l Leg) Name() string {
	return filepath.Base(l.Repo)
}

func (l Leg) Dir() string {
	if l.Worktree != "" {
		return l.Worktree
	}
	return l.Source
}

// Edge means From must merge before To. Both are Leg.Repo values.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Change struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	// Ticket is the Jira browse link the change was started from.
	Ticket    string    `json:"ticket,omitempty"`
	Branch    string    `json:"branch"`
	Body      string    `json:"body,omitempty"`
	Reviewers []string  `json:"reviewers,omitempty"`
	Created   time.Time `json:"created"`
	// Worktrees makes each leg a worktree under the change's directory instead
	// of the branch checked out in the repo's clone. Set when the change is
	// created; legs added later follow it.
	Worktrees bool   `json:"worktrees,omitempty"`
	Legs      []Leg  `json:"legs"`
	Declared  []Edge `json:"declared,omitempty"`
}

// UsesWorktrees reports whether the change's legs are worktrees: set at
// creation, or true of every leg of a change from before that setting.
func (c *Change) UsesWorktrees() bool {
	if c.Worktrees {
		return true
	}
	for _, l := range c.Legs {
		if l.Worktree == "" {
			return false
		}
	}
	return len(c.Legs) > 0
}

// Leg finds a leg by "vendor/repo" or bare repo name.
func (c *Change) Leg(name string) (*Leg, error) {
	for i := range c.Legs {
		if c.Legs[i].Repo == name || c.Legs[i].Name() == name {
			return &c.Legs[i], nil
		}
	}
	return nil, fmt.Errorf("change %s has no leg %q", c.ID, name)
}

func (c *Change) AddDeclared(e Edge) {
	if !slices.Contains(c.Declared, e) {
		c.Declared = append(c.Declared, e)
	}
}

// RemoveDeclared drops a declared edge, reporting whether it was there.
func (c *Change) RemoveDeclared(e Edge) bool {
	i := slices.Index(c.Declared, e)
	if i < 0 {
		return false
	}
	c.Declared = slices.Delete(c.Declared, i, i+1)
	return true
}

// RemoveLeg drops a leg and any declared edge naming it. The leg's branch and
// commits stay in its repo.
func (c *Change) RemoveLeg(name string) (Leg, error) {
	l, err := c.Leg(name)
	if err != nil {
		return Leg{}, err
	}
	removed := *l
	c.Legs = slices.DeleteFunc(c.Legs, func(x Leg) bool { return x.Repo == removed.Repo })
	c.Declared = slices.DeleteFunc(c.Declared, func(e Edge) bool { return e.From == removed.Repo || e.To == removed.Repo })
	return removed, nil
}

var idPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateID keeps IDs usable as both a directory path and a git branch.
// A '/' nests the ID, as in feature/login.
func ValidateID(id string) error {
	for _, part := range strings.Split(id, "/") {
		if !idPart.MatchString(part) || strings.Contains(part, "..") || strings.HasSuffix(part, ".lock") {
			return fmt.Errorf("invalid change id %q: use letters, digits, '.', '_' or '-', with '/' between parts", id)
		}
	}
	return nil
}

// Store keeps each change at <Home>/<id>/change.json.
type Store struct {
	Home string
}

const file = "change.json"

func (s Store) Dir(id string) string {
	return filepath.Join(s.Home, filepath.FromSlash(id))
}

// CheckNew refuses an ID that would nest inside an existing change's
// directory, or whose directory already holds something, such as changes
// nested under it. Git refuses the same overlap between branches.
func (s Store) CheckNew(id string) error {
	parts := strings.Split(id, "/")
	for i := 1; i < len(parts); i++ {
		if p := strings.Join(parts[:i], "/"); s.Exists(p) {
			return fmt.Errorf("change %s can't sit inside change %s", id, p)
		}
	}
	if _, err := os.Stat(s.Dir(id)); err == nil {
		return fmt.Errorf("%s already exists, so change %s can't use it", s.Dir(id), id)
	}
	return nil
}

func (s Store) Exists(id string) bool {
	if ValidateID(id) != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(s.Dir(id), file))
	return err == nil
}

// Load returns an error wrapping fs.ErrNotExist when the change is unknown.
func (s Store) Load(id string) (*Change, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(s.Dir(id), file))
	if err != nil {
		return nil, err
	}
	var c Change
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("read change %s: %w", id, err)
	}
	return &c, nil
}

func (s Store) Save(c *Change) error {
	dir := s.Dir(c.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSON(dir, file, c)
}

// writeJSON replaces dir/name in one rename, so a reader never sees half a file.
func writeJSON(dir, name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// orderFile at Home ranks changes, first to last, as the user arranged them.
const orderFile = "order.json"

// Order is the saved ranking of change IDs; it may name changes since cleaned.
func (s Store) Order() []string {
	var ids []string
	data, err := os.ReadFile(filepath.Join(s.Home, orderFile))
	if err == nil {
		json.Unmarshal(data, &ids)
	}
	return ids
}

func (s Store) SetOrder(ids []string) error {
	if err := os.MkdirAll(s.Home, 0o755); err != nil {
		return err
	}
	return writeJSON(s.Home, orderFile, ids)
}

// List returns every change in the saved order. Changes it doesn't rank, such
// as ones started since, come first, newest first.
func (s Store) List() ([]*Change, error) {
	var out []*Change
	err := filepath.WalkDir(s.Home, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			if path == s.Home && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if !e.IsDir() || path == s.Home {
			return nil
		}
		rel, _ := filepath.Rel(s.Home, path)
		id := filepath.ToSlash(rel)
		if ValidateID(id) != nil {
			return fs.SkipDir
		}
		if !s.Exists(id) {
			return nil
		}
		c, err := s.Load(id)
		if err != nil {
			return err
		}
		out = append(out, c)
		// A change's directory holds its worktrees, never other changes.
		return fs.SkipDir
	})
	if err != nil {
		return nil, err
	}
	rank := map[string]int{}
	for i, id := range s.Order() {
		if _, dup := rank[id]; !dup {
			rank[id] = i
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ri, iok := rank[out[i].ID]
		rj, jok := rank[out[j].ID]
		if iok != jok {
			return !iok
		}
		if iok {
			return ri < rj
		}
		return out[i].Created.After(out[j].Created)
	})
	return out, nil
}

// Current picks the change for a command given no ID: the one whose directory
// contains cwd, else the only change there is.
func (s Store) Current(cwd string) (*Change, error) {
	if rel, err := filepath.Rel(s.Home, cwd); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for i := 1; i <= len(parts); i++ {
			if id := strings.Join(parts[:i], "/"); s.Exists(id) {
				return s.Load(id)
			}
		}
	}
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	switch len(all) {
	case 0:
		return nil, errors.New("no changes yet: run td start <ID> <repo>...")
	case 1:
		return all[0], nil
	}
	ids := make([]string, len(all))
	for i, c := range all {
		ids[i] = c.ID
	}
	return nil, fmt.Errorf("several changes exist, name one: %s", strings.Join(ids, ", "))
}

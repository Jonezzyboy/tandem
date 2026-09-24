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
	Repo     string `json:"repo"`
	Source   string `json:"source"`
	Worktree string `json:"worktree"`
	Base     string `json:"base"`
	BaseRef  string `json:"baseRef"`
	PR       int    `json:"pr,omitempty"`
	PRURL    string `json:"prUrl,omitempty"`
}

// Name is the repo name without its vendor, used for worktree dirs and CLI args.
func (l Leg) Name() string {
	return filepath.Base(l.Repo)
}

// Edge means From must merge before To. Both are Leg.Repo values.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Change struct {
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	Branch    string    `json:"branch"`
	Body      string    `json:"body,omitempty"`
	Reviewers []string  `json:"reviewers,omitempty"`
	Created   time.Time `json:"created"`
	Legs      []Leg     `json:"legs"`
	Declared  []Edge    `json:"declared,omitempty"`
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

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateID keeps IDs usable as both a directory name and a git branch.
func ValidateID(id string) error {
	if !idPattern.MatchString(id) || strings.Contains(id, "..") || strings.HasSuffix(id, ".lock") {
		return fmt.Errorf("invalid change id %q: use letters, digits, '.', '_' or '-'", id)
	}
	return nil
}

// Store keeps each change at <Home>/<id>/change.json, beside its worktrees.
type Store struct {
	Home string
}

const file = "change.json"

func (s Store) Dir(id string) string {
	return filepath.Join(s.Home, id)
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
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".change-*.json")
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
	return os.Rename(tmp.Name(), filepath.Join(dir, file))
}

// List returns every change, newest first.
func (s Store) List() ([]*Change, error) {
	entries, err := os.ReadDir(s.Home)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Change
	for _, e := range entries {
		if !e.IsDir() || !s.Exists(e.Name()) {
			continue
		}
		c, err := s.Load(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Current picks the change for a command given no ID: the one whose directory
// contains cwd, else the only change there is.
func (s Store) Current(cwd string) (*Change, error) {
	if rel, err := filepath.Rel(s.Home, cwd); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		id := strings.Split(rel, string(filepath.Separator))[0]
		if s.Exists(id) {
			return s.Load(id)
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

// Package graph infers merge order between a change's legs from their
// dependency manifests.
package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

type Kind string

const (
	Go       Kind = "go"
	Composer Kind = "composer"
	NPM      Kind = "npm"
)

// Package is one identity in a manifest. Dir is the manifest's directory
// relative to the repo root ("" for the root).
type Package struct {
	Name string
	Kind Kind
	Dir  string
}

// Manifest lists the package identities a repo publishes and consumes, across
// go.mod, composer.json and package.json.
type Manifest struct {
	Provides []Package
	Requires []Package
}

// ReadManifest scans dir and its immediate subdirectories, so monorepos laid
// out as backend/ + frontend/ are covered.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	dirs := []string{dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return m, err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() && !strings.HasPrefix(n, ".") && n != "vendor" && n != "node_modules" {
			dirs = append(dirs, filepath.Join(dir, n))
		}
	}
	var errs []error
	for _, d := range dirs {
		rel, _ := filepath.Rel(dir, d)
		if rel == "." {
			rel = ""
		}
		errs = append(errs, m.readGo(d, rel), m.readComposer(d, rel), m.readNPM(d, rel))
	}
	return m, errors.Join(errs...)
}

func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func (m *Manifest) readGo(dir, rel string) error {
	path := filepath.Join(dir, "go.mod")
	data, err := readOptional(path)
	if err != nil || data == nil {
		return err
	}
	f, err := modfile.ParseLax(path, data, nil)
	if err != nil {
		return err
	}
	if f.Module != nil {
		m.Provides = append(m.Provides, Package{f.Module.Mod.Path, Go, rel})
	}
	for _, r := range f.Require {
		m.Requires = append(m.Requires, Package{r.Mod.Path, Go, rel})
	}
	return nil
}

func (m *Manifest) readComposer(dir, rel string) error {
	path := filepath.Join(dir, "composer.json")
	data, err := readOptional(path)
	if err != nil || data == nil {
		return err
	}
	var c struct {
		Name       string                     `json:"name"`
		Require    map[string]json.RawMessage `json:"require"`
		RequireDev map[string]json.RawMessage `json:"require-dev"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if c.Name != "" {
		m.Provides = append(m.Provides, Package{c.Name, Composer, rel})
	}
	for _, deps := range []map[string]json.RawMessage{c.Require, c.RequireDev} {
		for name := range deps {
			m.Requires = append(m.Requires, Package{name, Composer, rel})
		}
	}
	return nil
}

func (m *Manifest) readNPM(dir, rel string) error {
	path := filepath.Join(dir, "package.json")
	data, err := readOptional(path)
	if err != nil || data == nil {
		return err
	}
	var p struct {
		Name             string            `json:"name"`
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if p.Name != "" {
		m.Provides = append(m.Provides, Package{p.Name, NPM, rel})
	}
	for _, deps := range []map[string]string{p.Dependencies, p.DevDependencies, p.PeerDependencies} {
		for name := range deps {
			m.Requires = append(m.Requires, Package{name, NPM, rel})
		}
	}
	return nil
}

type Node struct {
	Name     string
	Manifest Manifest
}

// Edge means From merges before To. Via is the package that links them, Kind
// its ecosystem and Dir the directory in To whose manifest requires it; all
// empty for a declared edge.
type Edge struct {
	From, To string
	Via      string
	Kind     Kind
	Dir      string
}

func Infer(nodes []Node) []Edge {
	var edges []Edge
	for _, a := range nodes {
		for _, b := range nodes {
			if a.Name == b.Name {
				continue
			}
		provides:
			for _, p := range a.Manifest.Provides {
				for _, r := range b.Manifest.Requires {
					if r.Name == p.Name && r.Kind == p.Kind {
						edges = append(edges, Edge{From: a.Name, To: b.Name, Via: p.Name, Kind: p.Kind, Dir: r.Dir})
						break provides
					}
				}
			}
		}
	}
	return edges
}

// Levels assigns each node its merge wave: 1 for nodes with no upstream, else
// one more than its deepest upstream. Edges naming unknown nodes are ignored.
func Levels(names []string, edges []Edge) (map[string]int, error) {
	known := make(map[string]bool, len(names))
	for _, n := range names {
		known[n] = true
	}
	indeg := map[string]int{}
	next := map[string][]string{}
	seen := map[[2]string]bool{}
	for _, e := range edges {
		k := [2]string{e.From, e.To}
		if !known[e.From] || !known[e.To] || seen[k] {
			continue
		}
		seen[k] = true
		indeg[e.To]++
		next[e.From] = append(next[e.From], e.To)
	}
	level := make(map[string]int, len(names))
	var queue []string
	for _, n := range names {
		if indeg[n] == 0 {
			level[n] = 1
			queue = append(queue, n)
		}
	}
	done := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		done++
		for _, m := range next[n] {
			level[m] = max(level[m], level[n]+1)
			if indeg[m]--; indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if done < len(names) {
		var cyclic []string
		for _, n := range names {
			if indeg[n] > 0 {
				cyclic = append(cyclic, n)
			}
		}
		sort.Strings(cyclic)
		return nil, fmt.Errorf("dependency cycle between %s", strings.Join(cyclic, ", "))
	}
	return level, nil
}

// Language names a repo's main ecosystem for choosing an editor: "go", "php",
// "js", or "" when there is no manifest. Root manifests win over those in
// immediate subdirectories, and Go over PHP over JS at each depth, so a PHP
// backend with a JS frontend beside it is "php".
func Language(dir string) string {
	order := []struct{ file, lang string }{{"go.mod", "go"}, {"composer.json", "php"}, {"package.json", "js"}}
	for _, o := range order {
		if _, err := os.Stat(filepath.Join(dir, o.file)); err == nil {
			return o.lang
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, o := range order {
		for _, e := range entries {
			n := e.Name()
			if !e.IsDir() || strings.HasPrefix(n, ".") || n == "vendor" || n == "node_modules" {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, n, o.file)); err == nil {
				return o.lang
			}
		}
	}
	return ""
}

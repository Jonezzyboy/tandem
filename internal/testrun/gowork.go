package testrun

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"go/version"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/graph"
	"golang.org/x/mod/modfile"
)

// LinkGo writes a go.work beside each Go module in s that requires a module
// another repo of s provides, so builds there use that repo's checkout instead
// of the version go.mod pins. go.work is untracked, so the repos stay clean;
// End removes it. It returns a note per repo it linked or had to skip.
func LinkGo(ctx context.Context, s *Session) map[string]string {
	type module struct {
		repo int
		dir  string // relative to the repo
	}
	provided := map[string]module{}
	manifests := make([]graph.Manifest, len(s.Repos))
	for i, r := range s.Repos {
		manifests[i], _ = graph.ReadManifest(r.Dir)
		for _, p := range manifests[i].Provides {
			if p.Kind == graph.Go {
				provided[p.Name] = module{i, p.Dir}
			}
		}
	}
	abs := func(m module) string { return filepath.Join(s.Repos[m.repo].Dir, m.dir) }
	involved := map[module]bool{}
	notes := map[string]string{}
	for i := range s.Repos {
		r := &s.Repos[i]
		uses := map[string][]module{} // module dir in r → modules it should use
		for _, req := range manifests[i].Requires {
			if m, ok := provided[req.Name]; ok && m.repo != i {
				uses[req.Dir] = append(uses[req.Dir], m)
			}
		}
		for _, d := range r.Workspaces {
			if _, keep := uses[d]; !keep {
				removeWork(filepath.Join(r.Dir, d))
			}
		}
		var written, skipped, linked []string
		for d, mods := range uses {
			// A go.work at the root covers the repo's other modules too, and Go
			// refuses to build in one it doesn't list.
			if d == "" {
				for _, p := range manifests[i].Provides {
					if p.Kind == graph.Go && p.Dir != "" {
						mods = append(mods, module{i, p.Dir})
					}
				}
			}
			dir := filepath.Join(r.Dir, d)
			if why := workBlocked(dir, slices.Contains(r.Workspaces, d)); why != "" {
				skipped = append(skipped, why)
				continue
			}
			dirs := make([]string, len(mods))
			for j, m := range mods {
				dirs[j] = abs(m)
			}
			if err := writeWork(dir, s.Key, dirs); err != nil {
				skipped = append(skipped, "go.work: "+err.Error())
				continue
			}
			written = append(written, d)
			involved[module{i, d}] = true
			for _, m := range mods {
				involved[m] = true
				if m.repo != i {
					linked = append(linked, path.Base(s.Repos[m.repo].Name))
				}
			}
		}
		slices.Sort(written)
		r.Workspaces = written
		var note []string
		if len(written) > 0 {
			slices.Sort(linked)
			note = append(note, "using local "+strings.Join(slices.Compact(linked), ", ")+" via go.work")
		}
		note = append(note, skipped...)
		if len(note) > 0 {
			notes[r.Name] = strings.Join(note, " · ")
		}
	}
	for m := range involved {
		r := &s.Repos[m.repo]
		if !slices.Contains(r.GoClean, m.dir) && manifestsClean(ctx, r.Dir, m.dir) {
			r.GoClean = append(r.GoClean, m.dir)
		}
	}
	return notes
}

// manifestsClean reports whether go.mod and go.sum in dir (relative to the
// repo) match HEAD.
func manifestsClean(ctx context.Context, repo, dir string) bool {
	out, err := gitx.Run(ctx, repo, append([]string{"status", "--porcelain", "--"}, manifestPaths(dir)...)...)
	return err == nil && out == ""
}

func manifestPaths(dir string) []string {
	return []string{path.Join(filepath.ToSlash(dir), "go.mod"), path.Join(filepath.ToSlash(dir), "go.sum")}
}

// unlinkGo removes the go.work files LinkGo wrote in r and puts back any
// go.mod or go.sum that was clean when linked, as go commands in workspace
// mode can rewrite them. It reports the module dirs it put back.
func unlinkGo(ctx context.Context, r Repo) ([]string, error) {
	for _, d := range r.Workspaces {
		if err := removeWork(filepath.Join(r.Dir, d)); err != nil {
			return nil, err
		}
	}
	var restored []string
	for _, d := range r.GoClean {
		if manifestsClean(ctx, r.Dir, d) {
			continue
		}
		tracked, err := gitx.Run(ctx, r.Dir, append([]string{"ls-files", "--"}, manifestPaths(d)...)...)
		if err != nil {
			return restored, err
		}
		if tracked != "" {
			if _, err := gitx.Run(ctx, r.Dir, append([]string{"checkout", "--quiet", "HEAD", "--"}, strings.Split(tracked, "\n")...)...); err != nil {
				return restored, err
			}
		}
		restored = append(restored, cmp.Or(d, "the root"))
	}
	return restored, nil
}

// workBlocked says why a go.work can't go in module dir mod: one Tandem
// didn't write is there, or a vendor directory, which workspace mode ignores.
func workBlocked(mod string, ours bool) string {
	name := filepath.Base(mod)
	if _, err := os.Stat(filepath.Join(mod, "go.work")); err == nil && !ours {
		return name + " already has a go.work, so it uses the pinned versions"
	}
	if _, err := os.Stat(filepath.Join(mod, "vendor")); err == nil {
		return name + " vendors its modules, so it uses the pinned versions"
	}
	return ""
}

func writeWork(mod, key string, dirs []string) error {
	goVersion := moduleGo(mod)
	lines := []string{"\t."}
	for _, d := range dirs {
		if v := moduleGo(d); version.Compare("go"+v, "go"+goVersion) > 0 {
			goVersion = v
		}
		rel, err := filepath.Rel(mod, d)
		if err != nil {
			return err
		}
		lines = append(lines, "\t"+filepath.ToSlash(rel))
	}
	slices.Sort(lines[1:])
	var b strings.Builder
	fmt.Fprintf(&b, "// Written by Tandem while testing %s; removed when testing ends.\n", key)
	if goVersion != "" {
		fmt.Fprintf(&b, "go %s\n", goVersion)
	}
	fmt.Fprintf(&b, "\nuse (\n%s\n)\n", strings.Join(slices.Compact(lines), "\n"))
	return os.WriteFile(filepath.Join(mod, "go.work"), []byte(b.String()), 0o644)
}

// moduleGo is the go directive of the module in dir, or "".
func moduleGo(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil || f.Go == nil {
		return ""
	}
	return f.Go.Version
}

// removeWork deletes the go.work Tandem wrote in dir and the go.work.sum go
// commands add beside it.
func removeWork(dir string) error {
	var errs []error
	for _, f := range []string{"go.work", "go.work.sum"} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

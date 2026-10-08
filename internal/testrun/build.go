package testrun

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// BuildTarget is a package in a repo with a build script.
type BuildTarget struct {
	Repo string `json:"repo"`
	// Dir is the package's directory, relative to the repo; "" is its root.
	Dir string `json:"dir"`
}

// buildDepth is how many directories deep packages are looked for, enough for
// layouts such as desktop/frontend.
const buildDepth = 3

var skipDirs = map[string]bool{"node_modules": true, ".git": true, "vendor": true, "dist": true, "build": true, ".next": true, ".svelte-kit": true}

// FindBuilds lists the packages under root whose package.json has a build
// script, shallowest first.
func FindBuilds(repo, root string) []BuildTarget {
	out := []BuildTarget{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.Count(rel, string(filepath.Separator)) >= buildDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "package.json" && hasBuildScript(path) {
			dir := filepath.ToSlash(filepath.Dir(rel))
			if dir == "." {
				dir = ""
			}
			out = append(out, BuildTarget{Repo: repo, Dir: dir})
		}
		return nil
	})
	slices.SortStableFunc(out, func(x, y BuildTarget) int { return depth(x.Dir) - depth(y.Dir) })
	return out
}

func depth(dir string) int {
	if dir == "" {
		return 0
	}
	return strings.Count(dir, "/") + 1
}

func hasBuildScript(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	return json.Unmarshal(data, &pkg) == nil && pkg.Scripts["build"] != ""
}

// Build installs the package in dir's dependencies and runs its build script,
// sending each line of output to onLine as it comes. npm is the command run;
// the zero value is "npm".
func Build(ctx context.Context, dir, npm string, onLine func(string)) error {
	if npm == "" {
		npm = "npm"
	}
	for _, args := range [][]string{{"install", "--no-audit", "--no-fund"}, {"run", "build"}} {
		onLine("$ npm " + strings.Join(args, " "))
		if err := stream(ctx, dir, npm, args, onLine); err != nil {
			return err
		}
	}
	return nil
}

func stream(ctx context.Context, dir, name string, args []string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	r, w := io.Pipe()
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			onLine(sc.Text())
		}
		io.Copy(io.Discard, r)
	})
	err := cmd.Wait()
	w.Close()
	wg.Wait()
	return err
}

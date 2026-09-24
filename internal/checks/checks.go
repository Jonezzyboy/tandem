// Package checks detects and runs each leg's local checks.
package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Check struct {
	Name string
	Dir  string
	Args []string
	// Skip, when set, is why the check cannot run in this worktree.
	Skip string
}

type Result struct {
	Check    Check
	Err      error
	Duration time.Duration
	Output   string
}

// Config is a repo's optional .tandem.yml. Listed checks replace detection.
type Config struct {
	Checks []struct {
		Name string `yaml:"name"`
		Dir  string `yaml:"dir"`
		Run  string `yaml:"run"`
	} `yaml:"checks"`
}

func Detect(root string) ([]Check, error) {
	data, err := os.ReadFile(filepath.Join(root, ".tandem.yml"))
	switch {
	case err == nil:
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf(".tandem.yml: %w", err)
		}
		if len(cfg.Checks) > 0 {
			out := make([]Check, 0, len(cfg.Checks))
			for _, c := range cfg.Checks {
				name := c.Name
				if name == "" {
					name = c.Run
				}
				out = append(out, Check{Name: name, Dir: filepath.Join(root, c.Dir), Args: []string{"sh", "-c", c.Run}})
			}
			return out, nil
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	dirs := []string{root}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() && !strings.HasPrefix(n, ".") && n != "vendor" && n != "node_modules" {
			dirs = append(dirs, filepath.Join(root, n))
		}
	}
	var out []Check
	for _, d := range dirs {
		prefix := ""
		if d != root {
			prefix = filepath.Base(d) + ": "
		}
		for _, c := range detectDir(d) {
			c.Name = prefix + c.Name
			out = append(out, c)
		}
	}
	return out, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectDir(dir string) []Check {
	var out []Check
	add := func(args ...string) {
		out = append(out, Check{Name: strings.Join(args, " "), Dir: dir, Args: args})
	}
	if exists(filepath.Join(dir, "go.mod")) {
		add("go", "build", "-o", "/dev/null", "./...")
		add("go", "vet", "./...")
		add("go", "test", "./...")
	}
	if exists(filepath.Join(dir, "composer.json")) {
		out = append(out, phpChecks(dir)...)
	}
	if exists(filepath.Join(dir, "package.json")) {
		out = append(out, jsChecks(dir)...)
	}
	if exists(filepath.Join(dir, "buf.yaml")) {
		c := Check{Name: "buf lint", Dir: dir, Args: []string{"buf", "lint"}}
		if _, err := exec.LookPath("buf"); err != nil {
			c.Skip = "buf not installed"
		}
		out = append(out, c)
	}
	return out
}

// Worktrees start without vendor/ or node_modules/, so tools installed by the
// package manager are reported as skipped rather than silently dropped.
func phpChecks(dir string) []Check {
	var out []Check
	missing := "vendor/ missing: run composer install"
	if exists(filepath.Join(dir, "phpstan.neon")) || exists(filepath.Join(dir, "phpstan.neon.dist")) {
		c := Check{Name: "phpstan", Dir: dir, Args: []string{"vendor/bin/phpstan", "analyse", "--no-progress"}}
		if !exists(filepath.Join(dir, "vendor/bin/phpstan")) {
			c.Skip = missing
		}
		out = append(out, c)
	}
	if exists(filepath.Join(dir, "phpunit.xml")) || exists(filepath.Join(dir, "phpunit.xml.dist")) {
		c := Check{Name: "phpunit", Dir: dir, Args: []string{"vendor/bin/phpunit"}}
		if !exists(filepath.Join(dir, "vendor/bin/phpunit")) {
			c.Skip = missing
		}
		out = append(out, c)
	}
	return out
}

func jsChecks(dir string) []Check {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var p struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &p) != nil {
		return nil
	}
	pm := "npm"
	switch {
	case exists(filepath.Join(dir, "pnpm-lock.yaml")):
		pm = "pnpm"
	case exists(filepath.Join(dir, "yarn.lock")):
		pm = "yarn"
	}
	var out []Check
	for _, script := range []string{"typecheck", "lint", "test"} {
		if _, ok := p.Scripts[script]; !ok {
			continue
		}
		c := Check{Name: pm + " run " + script, Dir: dir, Args: []string{pm, "run", script}}
		if !exists(filepath.Join(dir, "node_modules")) {
			c.Skip = "node_modules/ missing: run " + pm + " install"
		}
		out = append(out, c)
	}
	return out
}

const tailLines = 20

func Run(ctx context.Context, c Check) Result {
	start := time.Now()
	cmd := exec.CommandContext(ctx, c.Args[0], c.Args[1:]...)
	cmd.Dir = c.Dir
	// CI=true stops watch-by-default runners such as vitest from waiting forever.
	cmd.Env = append(os.Environ(), "CI=true")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	r := Result{Check: c, Err: err, Duration: time.Since(start)}
	if err != nil {
		r.Output = tail(buf.String(), tailLines)
	}
	return r
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

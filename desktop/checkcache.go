package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jonezzyboy/tandem/internal/gitx"
)

const passedFile = "checks-passed.json"

// passedChecks remembers, per change, the state each leg was in when all its
// checks last passed, so an unchanged leg can be skipped.
type passedChecks struct {
	mu   sync.Mutex
	path string
	legs map[string]string
}

func (a *App) passedChecks(id string) *passedChecks {
	p := &passedChecks{path: filepath.Join(a.store.Dir(id), passedFile), legs: map[string]string{}}
	if data, err := os.ReadFile(p.path); err == nil {
		json.Unmarshal(data, &p.legs)
	}
	return p
}

func (p *passedChecks) same(leg, state string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return state != "" && p.legs[leg] == state
}

// record keeps state as leg's last passing state, or forgets it when the
// checks didn't all pass.
func (p *passedChecks) record(leg, state string, passed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if passed && state != "" {
		p.legs[leg] = state
	} else {
		delete(p.legs, leg)
	}
	if data, err := json.Marshal(p.legs); err == nil {
		os.WriteFile(p.path, data, 0o644)
	}
}

// legState fingerprints a working tree: its commit and every uncommitted
// change, untracked files included. It's "" when git can't say.
func legState(ctx context.Context, dir string) string {
	head, err := gitx.Run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	status, err := gitx.Run(ctx, dir, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return ""
	}
	diff, err := gitx.Run(ctx, dir, "diff", "HEAD")
	if err != nil {
		return ""
	}
	h := sha256.New()
	for _, s := range []string{head, status, diff} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	for _, line := range strings.Split(status, "\n") {
		if len(line) > 3 && line[:2] == "??" {
			if data, err := os.ReadFile(filepath.Join(dir, line[3:])); err == nil {
				h.Write(data)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

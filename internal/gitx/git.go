// Package gitx runs the git CLI. Shelling out keeps behaviour identical to the
// user's own git (config, credentials, hooks) at the cost of a process per call.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

func RefExists(ctx context.Context, dir, ref string) bool {
	_, err := Run(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

func HasRemote(ctx context.Context, dir, name string) bool {
	_, err := Run(ctx, dir, "remote", "get-url", name)
	return err == nil
}

func Fetch(ctx context.Context, dir string) error {
	_, err := Run(ctx, dir, "fetch", "--quiet", "origin")
	return err
}

// Base is the branch a change's leg is cut from and merges into. Ref is what
// comparisons use: the remote-tracking ref when origin exists.
type Base struct {
	Branch string
	Ref    string
}

func DefaultBase(ctx context.Context, dir string) (Base, error) {
	if HasRemote(ctx, dir, "origin") {
		if out, err := Run(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
			b := strings.TrimPrefix(out, "origin/")
			return Base{Branch: b, Ref: "origin/" + b}, nil
		}
		for _, b := range []string{"main", "master"} {
			if RefExists(ctx, dir, "refs/remotes/origin/"+b) {
				return Base{Branch: b, Ref: "origin/" + b}, nil
			}
		}
	}
	for _, b := range []string{"main", "master"} {
		if RefExists(ctx, dir, "refs/heads/"+b) {
			return Base{Branch: b, Ref: b}, nil
		}
	}
	return Base{}, errors.New("cannot determine default branch: no origin/HEAD, main or master")
}

type Status struct {
	Ahead, Behind int
	// Dirty counts uncommitted files in the checkout, whatever branch it is on.
	Dirty int
	// Current is the checked-out branch, "" when HEAD is detached.
	Current string
}

// StatusOf compares branch (not HEAD) with baseRef, so it stays right while
// the checkout is on another branch.
func StatusOf(ctx context.Context, dir, branch, baseRef string) (Status, error) {
	var s Status
	out, err := Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return s, err
	}
	if out != "" {
		s.Dirty = strings.Count(out, "\n") + 1
	}
	s.Current = CurrentBranch(ctx, dir)
	ref := "refs/heads/" + branch
	if !RefExists(ctx, dir, ref) {
		ref = "HEAD"
	}
	counts, err := Run(ctx, dir, "rev-list", "--left-right", "--count", baseRef+"..."+ref)
	if err != nil {
		return s, err
	}
	if _, err := fmt.Sscanf(counts, "%d\t%d", &s.Behind, &s.Ahead); err != nil {
		return s, fmt.Errorf("parse rev-list counts %q: %w", counts, err)
	}
	return s, nil
}

func CurrentBranch(ctx context.Context, dir string) string {
	out, err := Run(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// Switch checks out branch, refusing a checkout with uncommitted files so
// work never silently follows a switch to another branch.
func Switch(ctx context.Context, dir, branch string) error {
	if CurrentBranch(ctx, dir) == branch {
		return nil
	}
	out, err := Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if out != "" {
		return fmt.Errorf("%d uncommitted files: commit or stash them first", strings.Count(out, "\n")+1)
	}
	_, err = Run(ctx, dir, "switch", "--quiet", branch)
	return err
}

// Package cli implements the td command.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/gitx"
	"github.com/jonezzyboy/tandem/internal/ui"
	"github.com/jonezzyboy/tandem/internal/workspace"
)

type env struct {
	store change.Store
	roots []string
	out   io.Writer
	ui    ui.Style
	cwd   string
}

type command struct {
	name, usage, summary string
	run                  func(ctx context.Context, e *env, args []string) error
}

var commands []command

func init() {
	commands = []command{
		{"start", "td start <ID> <repo>... [--title T]", "create a change, or add repos to one: a branch per repo, checked out", runStart},
		{"add", "td add [ID] <repo>...", "add repos to a change: each gets its branch, and the merge order updates", runAdd},
		{"remove", "td remove [ID] <leg>...", "take repos out of a change (their branches stay)", runRemove},
		{"switch", "td switch [ID] [--base]", "check out the change's branch in every repo, or with --base their main branch", runSwitch},
		{"status", "td status [ID]", "local and GitHub state of every leg, in merge order", runStatus},
		{"sync", "td sync [ID]", "fetch every leg and rebase clean ones onto their base", runSync},
		{"check", "td check [ID] [leg...]", "run each leg's local checks", runCheck},
		{"pr", "td pr [ID] [--title T] [--body B | --body-file F] [--draft] [--reviewer a,b] [--dry-run]", "push legs and open or update their PRs, cross-linked", runPR},
		{"pin", "td pin [ID] [--no-commit]", "point downstream Go legs at their upstream leg's pushed commit", runPin},
		{"merge", "td merge [ID] [--method squash|merge|rebase] [--dry-run] [--yes]", "merge the PRs in dependency order, re-pinning as it goes", runMerge},
		{"clean", "td clean [--yes]", "switch landed changes' repos back to base and delete their branches", runClean},
		{"link", "td link [ID] <upstream> <downstream>", "declare that upstream merges before downstream", runLink},
		{"unlink", "td unlink [ID] <upstream> <downstream>", "remove a declared merge-order edge", runUnlink},
		{"path", "td path [ID] [leg]", "print a change's directory or a leg's repo", runPath},
		{"list", "td list", "list changes", runList},
		{"version", "td version", "print the td version", runVersion},
	}
}

// Version is set at release build time with -ldflags "-X .../internal/cli.Version=...".
var Version = "dev"

func runVersion(ctx context.Context, e *env, args []string) error {
	fmt.Fprintln(e.out, "td "+Version)
	return nil
}

// errReported means the command already printed why it failed.
var errReported = errors.New("reported")

func usage(w io.Writer) {
	fmt.Fprintln(w, "td — work on one change across many repos\n\nUsage:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w, "\nRun td <command> -h for flags. TANDEM_ROOT sets where clones live (default ~/code),\nTANDEM_HOME where changes are recorded (default ~/code/.tandem).")
}

func Main(args []string) int {
	return Run(args, os.Stdout, os.Stderr)
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "--version" {
		args = []string{"version"}
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(stdout)
		return 0
	}
	for _, c := range commands {
		if c.name != args[0] {
			continue
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		cwd, _ := os.Getwd()
		e := &env{
			store: change.Store{Home: workspace.Home()},
			roots: workspace.Roots(),
			out:   stdout,
			cwd:   cwd,
		}
		if f, ok := stdout.(*os.File); ok {
			e.ui = ui.Detect(f)
		}
		if err := c.run(ctx, e, args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			if !errors.Is(err, errReported) {
				fmt.Fprintf(stderr, "td %s: %v\n", c.name, err)
			}
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "td: unknown command %q\n\n", args[0])
	usage(stderr)
	return 2
}

func newFlags(name, usageLine string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: "+usageLine)
		fs.PrintDefaults()
	}
	return fs
}

// parseArgs lets flags appear after positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// changeFrom treats the first positional as a change ID when one by that name
// exists, else infers the change from the working directory: a leg's repo
// (preferring the change whose branch it has checked out), the change's own
// directory, or the only change there is.
func (e *env) changeFrom(pos []string) (*change.Change, []string, error) {
	if len(pos) > 0 && e.store.Exists(pos[0]) {
		c, err := e.store.Load(pos[0])
		return c, pos[1:], err
	}
	if c := e.changeForDir(); c != nil {
		return c, pos, nil
	}
	c, err := e.store.Current(e.cwd)
	return c, pos, err
}

func (e *env) changeForDir() *change.Change {
	all, err := e.store.List()
	if err != nil {
		return nil
	}
	var matches []*change.Change
	var repo string
	for _, c := range all {
		for _, l := range c.Legs {
			if within(e.cwd, l.Dir()) {
				matches = append(matches, c)
				repo = l.Dir()
				break
			}
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	current := gitx.CurrentBranch(context.Background(), repo)
	for _, c := range matches {
		if c.Branch == current {
			return c
		}
	}
	return nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

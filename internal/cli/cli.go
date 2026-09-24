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
	"strings"

	"github.com/jonezzyboy/tandem/internal/change"
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
		{"start", "td start <ID> <repo>... [--title T]", "create a change, or add repos to one, with a worktree per repo", runStart},
		{"status", "td status [ID]", "local and GitHub state of every leg, in merge order", runStatus},
		{"sync", "td sync [ID]", "fetch every leg and rebase clean ones onto their base", runSync},
		{"check", "td check [ID] [leg...]", "run each leg's local checks", runCheck},
		{"pr", "td pr [ID] [--title T] [--body B | --body-file F] [--draft] [--reviewer a,b] [--dry-run]", "push legs and open or update their PRs, cross-linked", runPR},
		{"link", "td link [ID] <upstream> <downstream>", "declare that upstream merges before downstream", runLink},
		{"path", "td path [ID] [leg]", "print a change's directory or a leg's worktree", runPath},
		{"list", "td list", "list changes", runList},
	}
}

// errReported means the command already printed why it failed.
var errReported = errors.New("reported")

func usage(w io.Writer) {
	fmt.Fprintln(w, "td — work on one change across many repos\n\nUsage:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w, "\nRun td <command> -h for flags. TANDEM_ROOT sets where clones live (default ~/code),\nTANDEM_HOME where changes and worktrees go (default ~/code/.tandem).")
}

func Main(args []string) int {
	return Run(args, os.Stdout, os.Stderr)
}

func Run(args []string, stdout, stderr io.Writer) int {
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
// exists, else infers the change from the working directory.
func (e *env) changeFrom(pos []string) (*change.Change, []string, error) {
	if len(pos) > 0 && e.store.Exists(pos[0]) {
		c, err := e.store.Load(pos[0])
		return c, pos[1:], err
	}
	c, err := e.store.Current(e.cwd)
	return c, pos, err
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

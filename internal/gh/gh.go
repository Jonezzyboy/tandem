// Package gh talks to GitHub through the gh CLI, reusing the user's gh auth.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

func run(ctx context.Context, dir, stdin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gh %s: %s", args[0]+" "+args[1], msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Check is one entry of a PR's statusCheckRollup: a CheckRun (Name, Status,
// Conclusion) or a StatusContext (Context, State).
type Check struct {
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

type PR struct {
	Number            int     `json:"number"`
	URL               string  `json:"url"`
	State             string  `json:"state"`
	Title             string  `json:"title"`
	Body              string  `json:"body"`
	IsDraft           bool    `json:"isDraft"`
	ReviewDecision    string  `json:"reviewDecision"`
	StatusCheckRollup []Check `json:"statusCheckRollup"`
}

const viewFields = "number,url,state,title,body,isDraft,reviewDecision,statusCheckRollup"

// View looks a PR up by number or head branch. It returns nil, nil when the
// branch has no PR.
func View(ctx context.Context, dir, selector string) (*PR, error) {
	out, err := run(ctx, dir, "", "pr", "view", selector, "--json", viewFields)
	if err != nil {
		if strings.Contains(err.Error(), "no pull requests found") {
			return nil, nil
		}
		return nil, err
	}
	var pr PR
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return nil, fmt.Errorf("parse gh pr view: %w", err)
	}
	return &pr, nil
}

type Rollup struct {
	Pass, Fail, Pending int
	Failing             []string
}

func (pr *PR) Rollup() Rollup {
	var r Rollup
	for _, c := range pr.StatusCheckRollup {
		name := c.Name
		if name == "" {
			name = c.Context
		}
		switch {
		case c.State != "":
			switch c.State {
			case "SUCCESS":
				r.Pass++
			case "FAILURE", "ERROR":
				r.Fail++
				r.Failing = append(r.Failing, name)
			default:
				r.Pending++
			}
		case c.Status != "COMPLETED":
			r.Pending++
		case c.Conclusion == "SUCCESS" || c.Conclusion == "NEUTRAL" || c.Conclusion == "SKIPPED":
			r.Pass++
		default:
			r.Fail++
			r.Failing = append(r.Failing, name)
		}
	}
	return r
}

type CreateOpts struct {
	Base, Head, Title, Body string
	Draft                   bool
	Reviewers               []string
}

func Create(ctx context.Context, dir string, o CreateOpts) (int, string, error) {
	args := []string{"pr", "create", "--base", o.Base, "--head", o.Head, "--title", o.Title, "--body-file", "-"}
	if o.Draft {
		args = append(args, "--draft")
	}
	for _, r := range o.Reviewers {
		args = append(args, "--reviewer", r)
	}
	// An empty stdin would make gh open an editor, so always send something.
	body := o.Body
	if body == "" {
		body = "\n"
	}
	out, err := run(ctx, dir, body, args...)
	if err != nil {
		return 0, "", err
	}
	lines := strings.Split(out, "\n")
	u := strings.TrimSpace(lines[len(lines)-1])
	n, err := strconv.Atoi(u[strings.LastIndex(u, "/")+1:])
	if err != nil {
		return 0, "", fmt.Errorf("unexpected gh pr create output %q", out)
	}
	return n, u, nil
}

func EditBody(ctx context.Context, dir string, number int, body string) error {
	_, err := run(ctx, dir, body, "pr", "edit", strconv.Itoa(number), "--body-file", "-")
	return err
}

// Ref turns https://github.com/owner/repo/pull/12 into owner/repo#12, which
// GitHub renders as a cross-repo link.
func Ref(prURL string) string {
	u, err := url.Parse(prURL)
	if err != nil {
		return prURL
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" {
		return prURL
	}
	return parts[0] + "/" + parts[1] + "#" + parts[3]
}

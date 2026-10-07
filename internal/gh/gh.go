// Package gh talks to GitHub through the gh CLI, reusing the user's gh auth.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	DetailsURL string `json:"detailsUrl"`
}

type PR struct {
	Number            int     `json:"number"`
	URL               string  `json:"url"`
	State             string  `json:"state"`
	Title             string  `json:"title"`
	Body              string  `json:"body"`
	IsDraft           bool    `json:"isDraft"`
	ReviewDecision    string  `json:"reviewDecision"`
	HeadRefOid        string  `json:"headRefOid"`
	StatusCheckRollup []Check `json:"statusCheckRollup"`
	MergeCommit       *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}

// MergeSHA is the commit a merged PR landed as, or "" before it merges.
func (pr *PR) MergeSHA() string {
	if pr.MergeCommit == nil {
		return ""
	}
	return pr.MergeCommit.OID
}

const viewFields = "number,url,state,title,body,isDraft,reviewDecision,headRefOid,statusCheckRollup,mergeCommit"

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
	FailedChecks        []Check
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
				r.FailedChecks = append(r.FailedChecks, c)
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
			r.FailedChecks = append(r.FailedChecks, c)
		}
	}
	return r
}

var actionsJob = regexp.MustCompile(`/actions/runs/(\d+)/job/(\d+)`)

// ActionsJob pulls the run and job IDs from a GitHub Actions check's details
// URL; ok is false for checks from other CI.
func ActionsJob(detailsURL string) (runID, jobID string, ok bool) {
	m := actionsJob.FindStringSubmatch(detailsURL)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// FailedLog is the output of a GitHub Actions job's failed steps.
func FailedLog(ctx context.Context, dir, jobID string) (string, error) {
	return run(ctx, dir, "", "run", "view", "--job", jobID, "--log-failed")
}

// RerunFailed reruns only the failed jobs of a GitHub Actions run.
func RerunFailed(ctx context.Context, dir, runID string) error {
	_, err := run(ctx, dir, "", "run", "rerun", runID, "--failed")
	return err
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

// Merge merges a PR with method squash, merge or rebase. With a merge queue the
// PR is only queued; callers poll View for MERGED.
func Merge(ctx context.Context, dir string, number int, method string) error {
	switch method {
	case "squash", "merge", "rebase":
	default:
		return fmt.Errorf("unknown merge method %q: use squash, merge or rebase", method)
	}
	_, err := run(ctx, dir, "", "pr", "merge", strconv.Itoa(number), "--"+method)
	return err
}

func EditBody(ctx context.Context, dir string, number int, body string) error {
	_, err := run(ctx, dir, body, "pr", "edit", strconv.Itoa(number), "--body-file", "-")
	return err
}

func Ready(ctx context.Context, dir string, number int) error {
	_, err := run(ctx, dir, "", "pr", "ready", strconv.Itoa(number))
	return err
}

type SearchPR struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	IsDraft    bool      `json:"isDraft"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

// Search runs gh search prs with the given qualifiers, e.g. --review-requested=@me.
func Search(ctx context.Context, qualifiers ...string) ([]SearchPR, error) {
	args := append([]string{"search", "prs"}, qualifiers...)
	args = append(args, "--limit", "50", "--json", "number,title,url,isDraft,updatedAt,repository,author")
	out, err := run(ctx, "", "", args...)
	if err != nil {
		return nil, err
	}
	var prs []SearchPR
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, fmt.Errorf("parse gh search prs: %w", err)
	}
	return prs, nil
}

// Clone clones repo (owner/name) into dir with gh, so it uses gh's sign-in
// and protocol.
func Clone(ctx context.Context, repo, dir string) error {
	_, err := run(ctx, "", "", "repo", "clone", repo, dir, "--", "--quiet")
	return err
}

// OpenPR is an open pull request with its head branch and CI state.
type OpenPR struct {
	Repo    string
	Number  int
	URL     string
	Title   string
	Draft   bool
	Author  string
	Updated time.Time
	Branch  string
	// CI is the head commit's rollup: SUCCESS, FAILURE, ERROR, PENDING,
	// EXPECTED, or "" when it has no checks.
	CI string
}

const openPRsQuery = `query($q: String!, $after: String) {
  search(query: $q, type: ISSUE, first: 100, after: $after) {
    pageInfo { hasNextPage endCursor }
    nodes { ... on PullRequest {
      number url title isDraft updatedAt headRefName
      author { login }
      repository { nameWithOwner }
      commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
    } }
  }
}`

// openPRPages caps one owner's listing at 500 PRs.
const openPRPages = 5

// OpenPRs lists every open PR in repos owned by owner, in one GraphQL search
// per 100: far cheaper on GitHub's search limit than a search per branch.
func OpenPRs(ctx context.Context, owner string) ([]OpenPR, error) {
	var out []OpenPR
	after := ""
	for range openPRPages {
		args := []string{"api", "graphql", "-f", "query=" + openPRsQuery, "-f", "q=is:pr is:open archived:false user:" + owner}
		if after != "" {
			args = append(args, "-f", "after="+after)
		}
		data, err := run(ctx, "", "", args...)
		if err != nil {
			return out, err
		}
		var body struct {
			Data struct {
				Search struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Number      int       `json:"number"`
						URL         string    `json:"url"`
						Title       string    `json:"title"`
						IsDraft     bool      `json:"isDraft"`
						UpdatedAt   time.Time `json:"updatedAt"`
						HeadRefName string    `json:"headRefName"`
						Author      struct {
							Login string `json:"login"`
						} `json:"author"`
						Repository struct {
							NameWithOwner string `json:"nameWithOwner"`
						} `json:"repository"`
						Commits struct {
							Nodes []struct {
								Commit struct {
									StatusCheckRollup *struct {
										State string `json:"state"`
									} `json:"statusCheckRollup"`
								} `json:"commit"`
							} `json:"nodes"`
						} `json:"commits"`
					} `json:"nodes"`
				} `json:"search"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(data), &body); err != nil {
			return out, fmt.Errorf("parse open PRs: %w", err)
		}
		for _, n := range body.Data.Search.Nodes {
			if n.Number == 0 {
				continue
			}
			pr := OpenPR{Repo: n.Repository.NameWithOwner, Number: n.Number, URL: n.URL, Title: n.Title, Draft: n.IsDraft,
				Author: n.Author.Login, Updated: n.UpdatedAt, Branch: n.HeadRefName}
			if c := n.Commits.Nodes; len(c) > 0 && c[0].Commit.StatusCheckRollup != nil {
				pr.CI = c[0].Commit.StatusCheckRollup.State
			}
			out = append(out, pr)
		}
		page := body.Data.Search.PageInfo
		if !page.HasNextPage {
			break
		}
		after = page.EndCursor
	}
	return out, nil
}

type User struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// CurrentUser is the account gh is signed in as.
func CurrentUser(ctx context.Context) (User, error) {
	out, err := run(ctx, "", "", "api", "user")
	if err != nil {
		return User{}, err
	}
	var u User
	if err := json.Unmarshal([]byte(out), &u); err != nil {
		return User{}, fmt.Errorf("parse gh api user: %w", err)
	}
	return u, nil
}

package gh

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Reviewer is someone, or a team, a PR can be sent to for review. Login is
// "org/team" for a team. Recent counts the signed-in user's recent PRs in the
// repos that they reviewed or were asked to.
type Reviewer struct {
	Login  string `json:"login"`
	Name   string `json:"name"`
	Team   bool   `json:"team"`
	Recent int    `json:"recent"`
}

// Reviewers suggests reviewers for repos ("owner/name"): recent reviewers of
// the signed-in user's PRs there first, then their teams, then everyone who
// can be assigned. It's one GraphQL query for the repos and one per owner for
// teams, which only organisations have, so a failed team lookup is skipped.
func Reviewers(ctx context.Context, repos []string) ([]Reviewer, error) {
	if len(repos) == 0 {
		return []Reviewer{}, nil
	}
	data, err := run(ctx, "", "", "api", "graphql", "-f", "query="+reviewersQuery(repos))
	if err != nil {
		return nil, err
	}
	var body struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		return nil, fmt.Errorf("parse reviewers: %w", err)
	}
	byLogin := map[string]*Reviewer{}
	add := func(r Reviewer) *Reviewer {
		key := strings.ToLower(r.Login)
		if have := byLogin[key]; have != nil {
			if have.Name == "" {
				have.Name = r.Name
			}
			return have
		}
		byLogin[key] = &r
		return &r
	}
	var viewer struct{ Login string }
	json.Unmarshal(body.Data["viewer"], &viewer)
	// Only assignable people can be requested, which also drops review bots;
	// a repo listing a full page may have more, so then nobody is dropped.
	assignable, truncated := map[string]bool{}, false
	for i := range repos {
		var repo struct {
			AssignableUsers struct {
				Nodes []struct{ Login, Name string }
			}
		}
		json.Unmarshal(body.Data[fmt.Sprintf("r%d", i)], &repo)
		truncated = truncated || len(repo.AssignableUsers.Nodes) == assignablePage
		for _, u := range repo.AssignableUsers.Nodes {
			assignable[strings.ToLower(u.Login)] = true
			add(Reviewer{Login: u.Login, Name: u.Name})
		}
	}
	var recent struct {
		Nodes []struct {
			Reviews struct {
				Nodes []struct{ Author struct{ Login string } }
			}
			ReviewRequests struct {
				Nodes []struct {
					RequestedReviewer struct{ Login, Name, CombinedSlug string }
				}
			}
		}
	}
	json.Unmarshal(body.Data["recent"], &recent)
	for _, pr := range recent.Nodes {
		seen := map[string]bool{}
		bump := func(r Reviewer) {
			if r.Login == "" || seen[strings.ToLower(r.Login)] {
				return
			}
			seen[strings.ToLower(r.Login)] = true
			add(r).Recent++
		}
		for _, rv := range pr.Reviews.Nodes {
			bump(Reviewer{Login: rv.Author.Login})
		}
		for _, rq := range pr.ReviewRequests.Nodes {
			if r := rq.RequestedReviewer; r.CombinedSlug != "" {
				bump(Reviewer{Login: r.CombinedSlug, Name: r.Name, Team: true})
			} else {
				bump(Reviewer{Login: r.Login, Name: r.Name})
			}
		}
	}
	orgs := owners(repos)
	teams := make([][]Reviewer, len(orgs))
	var wg sync.WaitGroup
	for i, owner := range orgs {
		wg.Go(func() { teams[i], _ = orgTeams(ctx, owner) })
	}
	wg.Wait()
	for _, ts := range teams {
		for _, t := range ts {
			add(t)
		}
	}
	delete(byLogin, strings.ToLower(viewer.Login))
	if !truncated {
		for key, r := range byLogin {
			if !r.Team && !assignable[key] {
				delete(byLogin, key)
			}
		}
	}
	out := make([]Reviewer, 0, len(byLogin))
	for _, r := range byLogin {
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b Reviewer) int {
		if c := cmp.Compare(b.Recent, a.Recent); c != 0 {
			return c
		}
		if a.Team != b.Team {
			if a.Team {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(a.Login), strings.ToLower(b.Login))
	})
	return out, nil
}

const assignablePage = 100

func reviewersQuery(repos []string) string {
	var b strings.Builder
	b.WriteString("query {\n  viewer { login }\n")
	search := "is:pr author:@me sort:updated-desc"
	for i, r := range repos {
		owner, name, _ := strings.Cut(r, "/")
		fmt.Fprintf(&b, "  r%d: repository(owner: %q, name: %q) { assignableUsers(first: %d) { nodes { login name } } }\n", i, owner, name, assignablePage)
		search += " repo:" + r
	}
	fmt.Fprintf(&b, `  recent: search(query: %q, type: ISSUE, first: 30) {
    nodes { ... on PullRequest {
      reviews(first: 20) { nodes { author { login } } }
      reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login name } ... on Team { combinedSlug name } } } }
    } }
  }
}`, search)
	return b.String()
}

func orgTeams(ctx context.Context, org string) ([]Reviewer, error) {
	q := fmt.Sprintf(`query { organization(login: %q) { teams(first: 100) { nodes { combinedSlug name } } } }`, org)
	data, err := run(ctx, "", "", "api", "graphql", "-f", "query="+q)
	if err != nil {
		return nil, err
	}
	var body struct {
		Data struct {
			Organization *struct {
				Teams struct {
					Nodes []struct{ CombinedSlug, Name string }
				}
			}
		}
	}
	if err := json.Unmarshal([]byte(data), &body); err != nil || body.Data.Organization == nil {
		return nil, fmt.Errorf("%s has no teams", org)
	}
	out := make([]Reviewer, 0, len(body.Data.Organization.Teams.Nodes))
	for _, t := range body.Data.Organization.Teams.Nodes {
		out = append(out, Reviewer{Login: t.CombinedSlug, Name: t.Name, Team: true})
	}
	return out, nil
}

func owners(repos []string) []string {
	var out []string
	for _, r := range repos {
		owner, _, _ := strings.Cut(r, "/")
		if !slices.Contains(out, owner) {
			out = append(out, owner)
		}
	}
	return out
}

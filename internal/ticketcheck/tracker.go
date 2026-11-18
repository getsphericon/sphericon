package ticketcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ReadyLabel marks a ticket an agent may take; TriageLabel puts it back before the maintainer.
const (
	ReadyLabel  = "ready-for-agent"
	TriageLabel = "needs-triage"
)

// LoadADRs reads every ADR of a directory.
func LoadADRs(dir string) ([]ADR, error) {
	names, err := filepath.Glob(filepath.Join(dir, "[0-9][0-9][0-9][0-9]-*.md"))
	if err != nil {
		return nil, err
	}
	adrs := make([]ADR, 0, len(names))
	for _, name := range names {
		text, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		adr, err := ParseADR(name, string(text))
		if err != nil {
			return nil, err
		}
		adrs = append(adrs, adr)
	}
	return adrs, nil
}

// Tracker reads and labels the issues of the repository through the gh CLI (GH_TOKEN in CI).
type Tracker struct{}

type ghIssue struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	Body   string    `json:"body"`
	Labels []ghLabel `json:"labels"`
}

type ghLabel struct {
	Name string `json:"name"`
}

func (g ghIssue) ready() bool {
	return slices.ContainsFunc(g.Labels, func(l ghLabel) bool { return l.Name == ReadyLabel })
}

// Tickets returns how each issue stands, open, closed as completed or closed as not planned (a
// number that is no issue counts as completed), and the sub-issues of every open ticket an ADR's
// `build:` lists, read in one query.
func (Tracker) Tickets(ctx context.Context, adrs []ADR) (Tickets, error) {
	states, err := issueStates(ctx)
	if err != nil {
		return nil, err
	}
	t := tracked{states: states, subIssues: map[int][]int{}}
	var listed []int
	for _, adr := range adrs {
		for _, n := range adr.Build.Tickets {
			if t.State(n) == Open && !slices.Contains(listed, n) {
				listed = append(listed, n)
			}
		}
	}
	if len(listed) == 0 {
		return t, nil
	}
	var query strings.Builder
	query.WriteString("query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {")
	for _, n := range listed {
		fmt.Fprintf(&query, " i%d: issue(number: %d) { subIssues(first: 100) { nodes { number } } }", n, n)
	}
	query.WriteString(" } }")
	out, err := gh(ctx, "api", "graphql", "-F", "owner={owner}", "-F", "name={repo}", "-f", "query="+query.String())
	if err != nil {
		return nil, err
	}
	var found struct {
		Data struct {
			Repository map[string]struct {
				SubIssues struct {
					Nodes []struct {
						Number int `json:"number"`
					} `json:"nodes"`
				} `json:"subIssues"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &found); err != nil {
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	for _, n := range listed {
		for _, node := range found.Data.Repository["i"+strconv.Itoa(n)].SubIssues.Nodes {
			t.subIssues[n] = append(t.subIssues[n], node.Number)
		}
	}
	return t, nil
}

type tracked struct {
	states    map[int]TicketState
	subIssues map[int][]int
}

func (t tracked) State(n int) TicketState {
	if state, ok := t.states[n]; ok {
		return state
	}
	return Completed
}

func (t tracked) SubIssues(n int) []int { return t.subIssues[n] }

func issueStates(ctx context.Context) (map[int]TicketState, error) {
	out, err := gh(ctx, "issue", "list", "--state", "all", "--limit", "100000", "--json", "number,state,stateReason",
		"--jq", `.[] | "\(.number) \(.state) \(.stateReason)"`)
	if err != nil {
		return nil, err
	}
	states := map[int]TicketState{}
	for line := range strings.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("gh issue list: %q is not a number", fields[0])
		}
		switch {
		case fields[1] == "OPEN":
			states[n] = Open
		case len(fields) > 2 && fields[2] == "NOT_PLANNED":
			states[n] = NotPlanned
		default:
			states[n] = Completed
		}
	}
	return states, nil
}

// Ready returns the open issues labelled ready-for-agent.
func (t Tracker) Ready(ctx context.Context) ([]Issue, error) {
	out, err := gh(ctx, "issue", "list", "--state", "open", "--label", ReadyLabel, "--limit", "10000",
		"--json", "number,title,body,labels")
	if err != nil {
		return nil, err
	}
	var found []ghIssue
	if err := json.Unmarshal([]byte(out), &found); err != nil {
		return nil, fmt.Errorf("gh issue list: %w", err)
	}
	issues := make([]Issue, 0, len(found))
	for _, g := range found {
		issue, err := t.withBlockers(ctx, g)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

// Issue returns one issue and whether it is labelled ready-for-agent.
func (t Tracker) Issue(ctx context.Context, number int) (Issue, bool, error) {
	out, err := gh(ctx, "issue", "view", strconv.Itoa(number), "--json", "number,title,body,labels")
	if err != nil {
		return Issue{}, false, err
	}
	var g ghIssue
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		return Issue{}, false, fmt.Errorf("gh issue view: %w", err)
	}
	issue, err := t.withBlockers(ctx, g)
	return issue, g.ready(), err
}

// withBlockers adds the issue's parent, its sub-issues and the native blocking edges of the issue
// and of its parent: a sub-issue waits for what its parent waits for.
func (Tracker) withBlockers(ctx context.Context, g ghIssue) (Issue, error) {
	issue := Issue{Number: g.Number, Title: g.Title, Body: g.Body}
	subIssues, err := numbers(ctx, g.Number, fmt.Sprintf("repos/{owner}/{repo}/issues/%d/sub_issues", g.Number))
	if err != nil {
		return Issue{}, err
	}
	issue.SubIssues = subIssues
	parent, err := gh(ctx, "api", fmt.Sprintf("repos/{owner}/{repo}/issues/%d", g.Number),
		"--jq", `.parent_issue_url // "" | split("/") | last`)
	if err != nil {
		return Issue{}, err
	}
	if parent = strings.TrimSpace(parent); parent != "" {
		if issue.Parent, err = strconv.Atoi(parent); err != nil {
			return Issue{}, fmt.Errorf("parent of #%d: %q is not a number", g.Number, parent)
		}
	}
	for _, n := range []int{issue.Number, issue.Parent} {
		if n == 0 {
			continue
		}
		blockers, err := nativeBlockers(ctx, n)
		if err != nil {
			return Issue{}, err
		}
		issue.BlockedBy = append(issue.BlockedBy, blockers...)
	}
	return issue, nil
}

func nativeBlockers(ctx context.Context, number int) ([]int, error) {
	return numbers(ctx, number, fmt.Sprintf("repos/{owner}/{repo}/issues/%d/dependencies/blocked_by", number))
}

// numbers reads the issue numbers of a list endpoint about an issue, every page of it.
func numbers(ctx context.Context, number int, endpoint string) ([]int, error) {
	out, err := gh(ctx, "api", "--paginate", endpoint, "--jq", ".[].number")
	if err != nil {
		return nil, err
	}
	var found []int
	for field := range strings.FieldsSeq(out) {
		n, err := strconv.Atoi(field)
		if err != nil {
			return nil, fmt.Errorf("%s of #%d: %q is not a number", endpoint, number, field)
		}
		found = append(found, n)
	}
	return found, nil
}

// TakeBack moves an issue from ready-for-agent to needs-triage and says why.
func (Tracker) TakeBack(ctx context.Context, number int, report Report) error {
	n := strconv.Itoa(number)
	if _, err := gh(ctx, "issue", "edit", n, "--remove-label", ReadyLabel, "--add-label", TriageLabel); err != nil {
		return err
	}
	_, err := gh(ctx, "issue", "comment", n, "--body", Comment(report))
	return err
}

func gh(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// OpenGraph reads every open issue with its labels, its parent and its edges to other open issues,
// in one paginated query.
func (Tracker) OpenGraph(ctx context.Context) ([]Node, error) {
	const query = `query($owner: String!, $name: String!, $endCursor: String) {
  repository(owner: $owner, name: $name) {
    issues(states: OPEN, first: 100, after: $endCursor) {
      pageInfo { hasNextPage endCursor }
      nodes {
        number title
        labels(first: 20) { nodes { name } }
        parent { number }
        blockedBy(first: 100) { nodes { number state } }
        subIssues(first: 100) { nodes { number state } }
      }
    }
  }
}`
	out, err := gh(ctx, "api", "graphql", "--paginate", "--slurp",
		"-F", "owner={owner}", "-F", "name={repo}", "-f", "query="+query)
	if err != nil {
		return nil, err
	}
	type ref struct {
		Number int    `json:"number"`
		State  string `json:"state"`
	}
	var pages []struct {
		Data struct {
			Repository struct {
				Issues struct {
					Nodes []struct {
						Number int    `json:"number"`
						Title  string `json:"title"`
						Labels struct {
							Nodes []ghLabel `json:"nodes"`
						} `json:"labels"`
						Parent    *ref `json:"parent"`
						BlockedBy struct {
							Nodes []ref `json:"nodes"`
						} `json:"blockedBy"`
						SubIssues struct {
							Nodes []ref `json:"nodes"`
						} `json:"subIssues"`
					} `json:"nodes"`
				} `json:"issues"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &pages); err != nil {
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	open := func(refs []ref) []int {
		var numbers []int
		for _, r := range refs {
			if r.State == "OPEN" {
				numbers = append(numbers, r.Number)
			}
		}
		return numbers
	}
	var nodes []Node
	for _, page := range pages {
		for _, issue := range page.Data.Repository.Issues.Nodes {
			node := Node{Number: issue.Number, Title: issue.Title,
				BlockedBy: open(issue.BlockedBy.Nodes), SubIssues: open(issue.SubIssues.Nodes)}
			for _, label := range issue.Labels.Nodes {
				node.Labels = append(node.Labels, label.Name)
			}
			if issue.Parent != nil {
				node.Parent = issue.Parent.Number
			}
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

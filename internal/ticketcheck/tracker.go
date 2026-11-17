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

// Open returns whether each issue number is an open issue.
func (Tracker) Open(ctx context.Context) (func(int) bool, error) {
	out, err := gh(ctx, "issue", "list", "--state", "open", "--limit", "10000", "--json", "number", "--jq", ".[].number")
	if err != nil {
		return nil, err
	}
	open := map[int]bool{}
	for field := range strings.FieldsSeq(out) {
		n, err := strconv.Atoi(field)
		if err != nil {
			return nil, fmt.Errorf("gh issue list: %q is not a number", field)
		}
		open[n] = true
	}
	return func(n int) bool { return open[n] }, nil
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

func (Tracker) withBlockers(ctx context.Context, g ghIssue) (Issue, error) {
	out, err := gh(ctx, "api", fmt.Sprintf("repos/{owner}/{repo}/issues/%d/dependencies/blocked_by", g.Number),
		"--jq", ".[].number")
	if err != nil {
		return Issue{}, err
	}
	issue := Issue{Number: g.Number, Title: g.Title, Body: g.Body}
	for field := range strings.FieldsSeq(out) {
		n, err := strconv.Atoi(field)
		if err != nil {
			return Issue{}, fmt.Errorf("blocked_by of #%d: %q is not a number", g.Number, field)
		}
		issue.BlockedBy = append(issue.BlockedBy, n)
	}
	return issue, nil
}

// TakeBack moves an issue from ready-for-agent to needs-triage and says why.
func (Tracker) TakeBack(ctx context.Context, number int, gaps []Gap) error {
	n := strconv.Itoa(number)
	if _, err := gh(ctx, "issue", "edit", n, "--remove-label", ReadyLabel, "--add-label", TriageLabel); err != nil {
		return err
	}
	_, err := gh(ctx, "issue", "comment", n, "--body", Comment(gaps))
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

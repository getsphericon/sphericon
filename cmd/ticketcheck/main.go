// Command ticketcheck runs the checks of internal/ticketcheck: that every ADR says whether it is
// built (`adrs`, part of `mise run check:adr`), and that a ticket ready for an agent is ready
// (`issues`, `mise run tickets:check`, and .github/workflows/tickets.yml with --apply).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/ticketcheck"
)

const (
	adrDir    = "docs/adr"
	flagApply = "apply"
)

func main() {
	if err := newRoot().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRoot() *cli.Command {
	return &cli.Command{
		Name:  "ticketcheck",
		Usage: "whether ADRs are built, and whether a ticket ready for an agent is ready: built ADRs, native edges, a sliced spec",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unknown command %q (see `ticketcheck --help`)", cmd.Args().First())
			}
			return errors.New("a command is required (see `ticketcheck --help`)")
		},
		Commands: []*cli.Command{
			{
				Name:  "adrs",
				Usage: "every ADR's `build:` field is valid, and every ADR from " + strconv.Itoa(ticketcheck.RecordedFrom) + " on has one",
				Action: func(context.Context, *cli.Command) error {
					adrs, err := ticketcheck.LoadADRs(adrDir)
					if err != nil {
						return err
					}
					return ticketcheck.CheckADRs(adrs)
				},
			},
			{
				Name:      "issues",
				Usage:     "the named issues, or every open ready-for-agent issue, build on built ADRs, carry a native edge for each `Blocked by` issue, and are sliced when a spec",
				ArgsUsage: "[<number>...]",
				Flags: []cli.Flag{&cli.BoolFlag{
					Name:  flagApply,
					Usage: "move a ready-for-agent issue that fails to needs-triage with a comment, instead of failing",
				}},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return checkIssues(ctx, cmd.Args().Slice(), cmd.Bool(flagApply))
				},
			},
			{
				Name: "graph",
				Usage: "every open issue with its native blockers, how many tickets it waits for, the edges to a " +
					"whole spec and the edges another edge already implies, longest wait first",
				Action: func(ctx context.Context, _ *cli.Command) error {
					nodes, err := ticketcheck.Tracker{}.OpenGraph(ctx)
					if err != nil {
						return err
					}
					rows := ticketcheck.Graph(nodes)
					for i := range rows {
						fmt.Println(rows[i])
					}
					return nil
				},
			},
		},
	}
}

func checkIssues(ctx context.Context, args []string, apply bool) error {
	adrs, err := ticketcheck.LoadADRs(adrDir)
	if err != nil {
		return err
	}
	tracker := ticketcheck.Tracker{}
	tickets, err := tracker.Tickets(ctx, adrs)
	if err != nil {
		return err
	}
	issues, err := selectIssues(ctx, tracker, args, apply)
	if err != nil {
		return err
	}
	failed := 0
	for _, issue := range issues {
		report := ticketcheck.Review(issue, adrs, tickets)
		if !report.Failed() {
			continue
		}
		failed++
		fmt.Printf("#%d %s\n", issue.Number, issue.Title)
		for _, line := range report.Lines() {
			fmt.Printf("  %s\n", line)
		}
		if apply {
			if err := tracker.TakeBack(ctx, issue.Number, report); err != nil {
				return err
			}
		}
	}
	if failed > 0 && !apply {
		return fmt.Errorf("%d ticket(s) not ready for an agent: wait for a ticket that builds each unbuilt ADR "+
			"(a native edge) or add a line `Without ADR <NNNN>: <why>`, give each `Blocked by` issue a native edge, "+
			"and slice a spec with `/to-tickets`", failed)
	}
	return nil
}

// selectIssues returns the named issues, or every open ready-for-agent issue. With --apply only a
// ready-for-agent issue is checked: the label is what the check takes back.
func selectIssues(ctx context.Context, tracker ticketcheck.Tracker, args []string, apply bool) ([]ticketcheck.Issue, error) {
	if len(args) == 0 {
		return tracker.Ready(ctx)
	}
	var issues []ticketcheck.Issue
	for _, arg := range args {
		number, err := strconv.Atoi(arg)
		if err != nil {
			return nil, fmt.Errorf("%q is not an issue number", arg)
		}
		issue, ready, err := tracker.Issue(ctx, number)
		if err != nil {
			return nil, err
		}
		if apply && !ready {
			continue
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

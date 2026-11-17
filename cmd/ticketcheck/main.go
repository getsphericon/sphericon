// Command ticketcheck runs the checks of internal/ticketcheck: that every ADR says whether it is
// built (`adrs`, part of `mise run check:adr`), and that a ticket ready for an agent builds on built
// ADRs (`issues`, `mise run tickets:check`, and .github/workflows/tickets.yml with --apply).
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
		Usage: "whether ADRs are built, and whether a ticket ready for an agent builds on built ADRs",
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
				Usage:     "the named issues, or every open ready-for-agent issue, build on built ADRs",
				ArgsUsage: "[<number>...]",
				Flags: []cli.Flag{&cli.BoolFlag{
					Name:  flagApply,
					Usage: "move a ready-for-agent issue that fails to needs-triage with a comment, instead of failing",
				}},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return checkIssues(ctx, cmd.Args().Slice(), cmd.Bool(flagApply))
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
	states, err := tracker.States(ctx)
	if err != nil {
		return err
	}
	issues, err := selectIssues(ctx, tracker, args, apply)
	if err != nil {
		return err
	}
	failed := 0
	for _, issue := range issues {
		gaps := ticketcheck.Check(issue, adrs, states)
		if len(gaps) == 0 {
			continue
		}
		failed++
		fmt.Printf("#%d %s\n", issue.Number, issue.Title)
		for _, gap := range gaps {
			fmt.Printf("  %s\n", gap)
		}
		if apply {
			if err := tracker.TakeBack(ctx, issue.Number, gaps); err != nil {
				return err
			}
		}
	}
	if failed > 0 && !apply {
		return fmt.Errorf("%d ticket(s) build on ADRs that are not built: wait for a ticket that builds each "+
			"(`Blocked by #<n>`) or add a line `Without ADR <NNNN>: <why>`", failed)
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

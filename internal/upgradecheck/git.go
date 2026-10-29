package upgradecheck

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Breaking reads the commits from base to HEAD and checks the pull request title, its body and
// the commits with CheckBreaking.
func Breaking(ctx context.Context, title, body, base string) error {
	out, err := output(ctx, "git", "log", "--no-merges", "--format=%B%x00", base+"..HEAD")
	if err != nil {
		return err
	}
	var commits []string
	for m := range strings.SplitSeq(out, "\x00") {
		if m = strings.TrimSpace(m); m != "" {
			commits = append(commits, m)
		}
	}
	return CheckBreaking(title, body, commits)
}

// Migrations lints the SQL migrations the pull request adds against base and checks them with
// CheckMigrations. It returns the files it linted: none when the pull request adds no migration.
func Migrations(ctx context.Context, title, base string) ([]string, error) {
	out, err := output(ctx, "git", "diff", "--name-only", "--diff-filter=A", base+"...HEAD", "--", "migrations")
	if err != nil {
		return nil, err
	}
	var added []string
	for line := range strings.FieldsSeq(out) {
		if strings.HasSuffix(line, ".sql") {
			added = append(added, line)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	findings, err := LintFiles(ctx, added)
	if err != nil {
		return nil, err
	}
	return added, CheckMigrations(title, findings)
}

// Immutable diffs the migrations against base and checks them with CheckImmutability. Renames are
// split into a delete and an add so the old name is reported.
func Immutable(ctx context.Context, body, base string) error {
	names := func(filter string) ([]string, error) {
		out, err := output(ctx, "git", "diff", "--name-only", "--no-renames", "--diff-filter="+filter, base+"...HEAD", "--", "migrations")
		return strings.Fields(out), err
	}
	modified, err := names("M")
	if err != nil {
		return err
	}
	deleted, err := names("D")
	if err != nil {
		return err
	}
	return CheckImmutability(modified, deleted, IsHistoryCollapse(body))
}

func output(ctx context.Context, name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const usage = `usage:
  upgradecheck breaking   --title T [--body-file F] --base REF
  upgradecheck migrations --title T --base REF
  upgradecheck immutable  [--body-file F] --base REF`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	title := fs.String("title", "", "pull request title (the squash commit subject)")
	bodyFile := fs.String("body-file", "", "file with the pull request body")
	base := fs.String("base", "origin/main", "ref the pull request is compared against")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "breaking":
		return runBreaking(*title, *bodyFile, *base)
	case "migrations":
		return runMigrations(*title, *base)
	case "immutable":
		return runImmutable(*bodyFile, *base)
	default:
		return errors.New(usage)
	}
}

func runBreaking(title, bodyFile, base string) error {
	var body string
	if bodyFile != "" {
		b, err := os.ReadFile(bodyFile)
		if err != nil {
			return err
		}
		body = string(b)
	}
	out, err := output("git", "log", "--no-merges", "--format=%B%x00", base+"..HEAD")
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

func runMigrations(title, base string) error {
	out, err := output("git", "diff", "--name-only", "--diff-filter=A", base+"...HEAD", "--", "migrations")
	if err != nil {
		return err
	}
	var added []string
	for line := range strings.FieldsSeq(out) {
		if strings.HasSuffix(line, ".sql") && !isRiver(filepath.Base(line)) {
			added = append(added, line)
		}
	}
	if len(added) == 0 {
		fmt.Println("no migration added; nothing to lint")
		return nil
	}
	findings, err := LintFiles(added)
	if err != nil {
		return err
	}
	return CheckMigrations(title, findings)
}

// LintFiles runs squawk over the migration files and returns its destructive findings.
// squawk exits non-zero when it reports a finding, with the report on stdout: the report is the
// verdict, so an exit error with a report is not a failure to run.
func LintFiles(paths []string) ([]Finding, error) {
	report, err := output("squawk", append([]string{"--reporter", "json"}, paths...)...)
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || !strings.HasPrefix(strings.TrimSpace(report), "[")) {
		return nil, err
	}
	return Destructive([]byte(report))
}

func output(name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

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
	"slices"
	"strconv"
	"strings"
)

const usage = `usage:
  upgradecheck breaking   --title T [--body-file F] --base REF
  upgradecheck migrations --title T --base REF`

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
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return err
	}
	all := make([]string, 0, len(files))
	for _, f := range files {
		all = append(all, filepath.Base(f))
	}
	slices.Sort(all)

	out, err := output("git", "diff", "--name-only", "--diff-filter=A", base+"...HEAD", "--", "migrations")
	if err != nil {
		return err
	}
	var added []string
	for line := range strings.FieldsSeq(out) {
		if strings.HasSuffix(line, ".sql") {
			added = append(added, filepath.Base(line))
		}
	}
	n := LintWindow(all, added)
	if n == 0 {
		fmt.Println("no migration added; nothing to lint")
		return nil
	}
	// atlas exits non-zero when it reports a finding, with the report on stdout: the report is
	// the verdict here, so an exit error with a report is not a failure to run.
	lint, err := output("atlas", "migrate", "lint", "--env", "local", "--latest", strconv.Itoa(n), "--format", "{{ json .Files }}")
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || !strings.HasPrefix(lint, "[")) {
		return err
	}
	findings, err := Destructive([]byte(lint))
	if err != nil {
		return err
	}
	return CheckMigrations(title, findings)
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

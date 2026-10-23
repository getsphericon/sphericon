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
		if strings.HasSuffix(line, ".sql") {
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

// LintFiles runs squawk over the Up section of each migration file and returns its destructive
// and blocking findings, named after the original files. The generated river migrations are
// skipped. Only the Up section is linted: the generator writes a Down that reverses the Up (a
// DROP COLUMN for an added column), and a Down is never applied in production.
// goose wraps a migration in a transaction unless it says `-- +goose NO TRANSACTION`, so squawk
// is told to assume one for the others: an index or constraint on a table that the same migration
// creates is then not a lock on a live table.
func LintFiles(paths []string) ([]Finding, error) {
	dir, err := os.MkdirTemp("", "upgradecheck-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	var inTx, noTx []string
	for _, p := range paths {
		if isRiver(filepath.Base(p)) {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		up := UpSection(string(src))
		dst := filepath.Join(dir, filepath.Base(p))
		if err := os.WriteFile(dst, []byte(up), 0o600); err != nil {
			return nil, err
		}
		if hasNoTransaction(up) {
			noTx = append(noTx, dst)
		} else {
			inTx = append(inTx, dst)
		}
	}
	var findings []Finding
	for _, run := range []struct {
		flag  string
		files []string
	}{{"--assume-in-transaction", inTx}, {"--no-assume-in-transaction", noTx}} {
		found, err := squawk(run.flag, run.files)
		if err != nil {
			return nil, err
		}
		findings = append(findings, found...)
	}
	return findings, nil
}

// squawk lints the files and returns their findings. It exits non-zero when it reports a finding,
// with the report on stdout: the report is the verdict, so an exit error with a report is not a
// failure to run.
func squawk(flagName string, files []string) ([]Finding, error) {
	if len(files) == 0 {
		return nil, nil
	}
	report, err := output("squawk", append([]string{"--reporter", "json", flagName}, files...)...)
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || !strings.HasPrefix(strings.TrimSpace(report), "[")) {
		return nil, err
	}
	return Findings([]byte(report))
}

// hasNoTransaction reports whether a goose migration opts out of its transaction.
func hasNoTransaction(sql string) bool {
	for line := range strings.Lines(sql) {
		if strings.TrimSpace(line) == "-- +goose NO TRANSACTION" {
			return true
		}
	}
	return false
}

// UpSection returns a goose migration up to its `-- +goose Down` marker.
func UpSection(sql string) string {
	var b strings.Builder
	for line := range strings.Lines(sql) {
		if strings.HasPrefix(strings.TrimSpace(line), "-- +goose Down") {
			break
		}
		b.WriteString(line)
	}
	return b.String()
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

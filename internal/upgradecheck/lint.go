package upgradecheck

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// LintFiles runs squawk over the Up section of each migration file and returns its destructive
// and blocking findings, named after the original files. The generated river migrations are
// skipped. Only the Up section is linted: the generator writes a Down that reverses the Up (a
// DROP COLUMN for an added column), and a Down is never applied in production.
// goose wraps a migration in a transaction unless it says `-- +goose NO TRANSACTION`, so squawk
// is told to assume one for the others: an index or constraint on a table that the same migration
// creates is then not a lock on a live table.
func LintFiles(ctx context.Context, paths []string) ([]Finding, error) {
	dir, err := os.MkdirTemp("", "upgradecheck-")
	if err != nil {
		return nil, err
	}
	defer func() { cleanup.Report("remove the scratch directory", os.RemoveAll(dir)) }()
	var inTx, noTx []string
	for _, p := range paths {
		if isRiver(filepath.Base(p)) {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		up := upSection(string(src))
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
		found, err := squawk(ctx, run.flag, run.files)
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
func squawk(ctx context.Context, flagName string, files []string) ([]Finding, error) {
	if len(files) == 0 {
		return nil, nil
	}
	report, err := output(ctx, "squawk", append([]string{"--reporter", "json", flagName}, files...)...)
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

// upSection returns a goose migration up to its `-- +goose Down` marker.
func upSection(sql string) string {
	var b strings.Builder
	for line := range strings.Lines(sql) {
		if strings.HasPrefix(strings.TrimSpace(line), "-- +goose Down") {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func isRiver(name string) bool { return strings.Contains(name, "_river_") }

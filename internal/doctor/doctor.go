// Package doctor is `sphericon doctor`: it reads the same configuration as the server and
// judges an Instance from the outside, one Check per thing a Self-hoster can get wrong, each
// failure with the fix. It reads across Workspaces (every Sending domain, recently sent mail),
// so it holds the raw ent client like the other Instance-wide readers.
//
// Every check that reaches beyond the process (SMTP, DNS, the Instance's own URL) goes through
// a seam on Input, so a test runs the lot without a network.
package doctor

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/sending"
)

// Postgres majors the Instance supports: the oldest we document, and the newest the test
// suite runs against. docs/self-hosting.md cites the same range.
const (
	MinPostgresMajor       = 14
	MaxTestedPostgresMajor = 18
)

// recentMail is how many of the newest sent messages the origin check reads.
const recentMail = 200

// Status is the verdict of one Check.
type Status string

const (
	Pass Status = "pass"
	// Warn is worth a look but does not fail the run.
	Warn Status = "warn"
	Fail Status = "fail"
	// Skip means the check does not apply to this Instance.
	Skip Status = "skip"
)

// Check is one line of the report. Fix is set on a Fail or Warn and says what to do.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Report is the outcome of a run.
type Report struct {
	Checks []Check `json:"checks"`
}

// Failed reports whether any check failed; warnings do not count.
func (r Report) Failed() bool {
	for _, c := range r.Checks {
		if c.Status == Fail {
			return true
		}
	}
	return false
}

func (r *Report) add(name string, status Status, detail, fix string) {
	r.Checks = append(r.Checks, Check{Name: name, Status: status, Detail: detail, Fix: fix})
}

// Input is what a run reads. Config is nil, with ConfigErr set, when the configuration did
// not load; DB and Ent are nil, with DBErr set, when the database could not be opened.
type Input struct {
	Config    *config.Config
	ConfigErr error

	DB    *sql.DB
	Ent   *ent.Client
	DBErr error

	Build buildinfo.Info
	Now   func() time.Time

	// DialSMTP connects to the system mail relay as the server would: EHLO, STARTTLS when
	// offered, and AUTH when a user is set.
	DialSMTP func(ctx context.Context, cfg *config.Config) error
	// HTTP fetches the Instance's own public URL.
	HTTP *http.Client
	// LicensePublicKey verifies SPHERICON_LICENSE_KEY: licensekey.ProductionKey outside tests.
	LicensePublicKey ed25519.PublicKey
	// LookupTXT resolves DNS, always the real resolver outside tests (never the dev stub).
	LookupTXT sending.TXTLookup
}

// Run performs every check, in the order the report prints them. A check that cannot run
// because an earlier one failed (no configuration, no database) is reported as failed too,
// so the report always lists the same checks.
func Run(ctx context.Context, in Input) Report {
	var r Report
	if in.ConfigErr != nil || in.Config == nil {
		r.add("configuration", Fail, fmt.Sprintf("the configuration did not load: %v", in.ConfigErr),
			"Set the variable named in the message (see the table in docs/self-hosting.md), then run `sphericon doctor` again.")
		return r
	}
	r.add("configuration", Pass, "the configuration loads", "")

	checkDatabase(ctx, &r, in)
	checkSMTP(ctx, &r, in)
	checkAppURL(ctx, &r, in)
	checkSecrets(&r, in)
	checkLicense(&r, in)
	checkDKIM(ctx, &r, in)
	checkMailOrigin(ctx, &r, in)
	return r
}

// WriteText prints the report for a person: one line per check, the fix indented under it.
func (r Report) WriteText(w io.Writer) {
	tags := map[Status]string{Pass: "[ ok ]", Warn: "[warn]", Fail: "[FAIL]", Skip: "[skip]"}
	for _, c := range r.Checks {
		_, _ = fmt.Fprintf(w, "%s %s: %s\n", tags[c.Status], c.Name, c.Detail)
		if c.Fix != "" {
			_, _ = fmt.Fprintf(w, "       fix: %s\n", c.Fix)
		}
	}
	failed, warned := 0, 0
	for _, c := range r.Checks {
		switch c.Status {
		case Fail:
			failed++
		case Warn:
			warned++
		}
	}
	_, _ = fmt.Fprintf(w, "\n%d checks, %d failed, %d warnings\n", len(r.Checks), failed, warned)
}

// WriteJSON prints the report for a monitor: {"ok": bool, "checks": [...]}.
func (r Report) WriteJSON(w io.Writer) error {
	return json.NewEncoder(w).Encode(struct {
		OK bool `json:"ok"`
		Report
	}{OK: !r.Failed(), Report: r})
}

// Redactor wraps w so that none of the secrets reaches it: every occurrence becomes
// "[redacted]". A value shorter than four characters is left alone, since replacing it would
// mangle ordinary text; the secrets doctor guards (keys, passwords, tokens) are far longer.
func Redactor(w io.Writer, secrets []string) io.Writer {
	var forms []string
	for _, s := range secrets {
		if len(strings.TrimSpace(s)) < 4 {
			continue
		}
		forms = append(forms, s)
		// The JSON report spells a quote or backslash escaped.
		if quoted, err := json.Marshal(s); err == nil {
			forms = append(forms, string(quoted[1:len(quoted)-1]))
		}
	}
	if len(forms) == 0 {
		return w
	}
	// A Replacer tries its pairs in order, so the longest goes first: a secret that contains
	// another (a URL and its password) is replaced whole.
	slices.SortFunc(forms, func(a, b string) int { return len(b) - len(a) })
	pairs := make([]string, 0, 2*len(forms))
	for _, f := range forms {
		pairs = append(pairs, f, "[redacted]")
	}
	return &redactor{w: w, r: strings.NewReplacer(pairs...)}
}

type redactor struct {
	w io.Writer
	r *strings.Replacer
}

func (r *redactor) Write(p []byte) (int, error) {
	if _, err := io.WriteString(r.w, r.r.Replace(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

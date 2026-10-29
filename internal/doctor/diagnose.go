package doctor

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/wneessen/go-mail"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/cleanup"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/sending"
)

// ErrChecksFailed is what Diagnose returns when a check failed: the report it printed already
// says what, so the caller only exits non-zero.
var ErrChecksFailed = errors.New("doctor: a check failed")

// Timeouts of a run: the whole run, the database probe and the SMTP dial.
const (
	runTimeout   = 2 * time.Minute
	probeTimeout = 10 * time.Second
	smtpTimeout  = 15 * time.Second
	httpTimeout  = 10 * time.Second
)

// Seams are the reaches outside the process that Diagnose makes, so a test swaps them.
type Seams struct {
	Now       func() time.Time
	DialSMTP  func(ctx context.Context, cfg *config.Config) error
	HTTP      *http.Client
	LookupTXT sending.TXTLookup
}

// DefaultSeams are the real ones: the clock, the system relay, the network and the host's
// resolver.
func DefaultSeams() Seams {
	return Seams{
		Now:       time.Now,
		DialSMTP:  dialSMTP,
		HTTP:      &http.Client{Timeout: httpTimeout},
		LookupTXT: net.DefaultResolver.LookupTXT,
	}
}

// Diagnose is `sphericon doctor`: it loads the configuration of env the way the server reads it,
// runs every check and prints the report to w, as JSON when asJSON. Every byte it prints passes
// through a Redactor that holds the configured secrets. ErrChecksFailed when a check failed.
func Diagnose(ctx context.Context, env string, s Seams, w io.Writer, asJSON bool) error {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	cfg, loadErr := config.Load(env)
	w = Redactor(w, config.SecretValues(cfg))
	in := Input{
		Config:           cfg,
		ConfigErr:        loadErr,
		Build:            buildinfo.Current(),
		Now:              s.Now,
		DialSMTP:         s.DialSMTP,
		HTTP:             s.HTTP,
		LookupTXT:        s.LookupTXT,
		LicensePublicKey: licensekey.VerifyingKey(cfg != nil && cfg.TrustsDevLicense()),
	}
	if cfg != nil {
		db, err := openProbe(ctx, cfg.DatabaseURL)
		if err != nil {
			in.DBErr = err
		} else {
			defer cleanup.Close(db, "database")
			in.DB, in.Ent = db, appdb.NewEntClient(db)
		}
	}

	report := Run(ctx, in)
	if asJSON {
		if err := report.WriteJSON(w); err != nil {
			return err
		}
	} else {
		report.WriteText(w)
	}
	if report.Failed() {
		return ErrChecksFailed
	}
	return nil
}

// openProbe opens a short-lived connection and proves it works, so an unreachable database
// is one failed check instead of an error in every query.
func openProbe(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		cleanup.Close(db, "database")
		return nil, err
	}
	return db, nil
}

// dialSMTP connects to the system relay the way a send does (STARTTLS when offered, AUTH when a
// user is set) and sends nothing.
func dialSMTP(ctx context.Context, cfg *config.Config) error {
	opts := []mail.Option{mail.WithPort(cfg.SMTPPort), mail.WithTLSPortPolicy(mail.TLSOpportunistic)}
	if cfg.SMTPUser != "" || cfg.SMTPPass != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(cfg.SMTPUser), mail.WithPassword(cfg.SMTPPass))
	}
	client, err := mail.NewClient(cfg.SMTPHost, opts...)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, smtpTimeout)
	defer cancel()
	if err := client.DialWithContext(ctx); err != nil {
		return err
	}
	return client.Close()
}

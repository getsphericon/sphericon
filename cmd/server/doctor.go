package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/urfave/cli/v3"
	"github.com/wneessen/go-mail"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/cleanup"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/doctor"
)

// errChecksFailed is what `doctor` returns when a check failed; main turns it into exit
// status 1 without logging, since the report already said everything.
var errChecksFailed = errors.New("doctor: a check failed")

// doctorSeams are the reaches outside the process that `doctor` makes, so a test swaps them.
type doctorSeams struct {
	dialSMTP  func(ctx context.Context, cfg *config.Config) error
	client    *http.Client
	lookupTXT func(ctx context.Context, name string) ([]string, error)
}

func newDoctorSeams() doctorSeams {
	return doctorSeams{
		dialSMTP:  dialSMTP,
		client:    &http.Client{Timeout: 10 * time.Second},
		lookupTXT: net.DefaultResolver.LookupTXT,
	}
}

// doctorCommand is `sphericon doctor [--json]`: it checks the Instance's configuration the way
// the server reads it and prints one line per check with a fix for each failure (exit 1 when
// any failed). Every byte of its output passes through a redactor that holds the secrets.
func doctorCommand(d deps) *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "check this Instance's configuration and say how to fix what is wrong",
		Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "print the report as JSON, for monitoring"}},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			cfg, loadErr := config.Load(d.env)
			w := doctor.Redactor(out(cmd), config.SecretValues(cfg))
			in := doctor.Input{
				Config:           cfg,
				ConfigErr:        loadErr,
				Build:            buildinfo.Current(),
				Now:              d.now,
				DialSMTP:         d.doctor.dialSMTP,
				HTTP:             d.doctor.client,
				LookupTXT:        d.doctor.lookupTXT,
				LicensePublicKey: licensekey.ProductionKey,
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

			report := doctor.Run(ctx, in)
			if cmd.Bool("json") {
				if err := report.WriteJSON(w); err != nil {
					return err
				}
			} else {
				report.WriteText(w)
			}
			if report.Failed() {
				return errChecksFailed
			}
			return nil
		},
	}
}

// openProbe opens a short-lived connection and proves it works, so an unreachable database
// is one failed check instead of an error in every query.
func openProbe(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.DialWithContext(ctx); err != nil {
		return err
	}
	return client.Close()
}

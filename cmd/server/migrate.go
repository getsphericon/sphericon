package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	appdb "github.com/getsphericon/sphericon/internal/db"
)

// migrateCommand is `sphericon migrate`: it applies the pending migrations with no time limit
// and exits (handy for an init container, release job or manual run), or with --status only
// prints what would run.
func migrateCommand(env string) *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "apply pending migrations and exit",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "status", Usage: "print the schema's state and the pending migrations; change nothing"},
			&cli.BoolFlag{Name: "json", Usage: "with --status, print the schema state and the table of Majors as JSON (what `upgrade` reads)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			status, asJSON := cmd.Bool("status"), cmd.Bool("json")
			if cmd.Args().Present() || (asJSON && !status) {
				return fmt.Errorf("unknown arguments %q (usage: migrate [--status [--json]])", cmd.Args().Slice())
			}
			cfg, err := config.Load(env)
			if err != nil {
				return err
			}
			if status {
				return appdb.WriteStatus(ctx, cfg.DatabaseURL, out(cmd), asJSON)
			}
			applied, err := appdb.ApplyMigrations(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			slog.Info("migrations applied", "count", applied)
			return nil
		},
	}
}

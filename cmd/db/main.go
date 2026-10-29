// Command db runs the development database tasks of internal/devdb: it creates and drops this
// checkout's database, writes the next goose migration from the ent schema and writes the
// migration for river's schema.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/devdb"
)

func main() {
	if err := newRoot().Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func newRoot() *cli.Command {
	return &cli.Command{
		Name:  "db",
		Usage: "database tasks of the dev workflow",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unknown command %q (see `db --help`)", cmd.Args().First())
			}
			return errors.New("a command is required (see `db --help`)")
		},
		Commands: []*cli.Command{
			{
				Name:  "create",
				Usage: "create this checkout's database",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return onDatabase(ctx, devdb.Create)
				},
			},
			{
				Name:  "drop",
				Usage: "drop this checkout's database",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return onDatabase(ctx, devdb.Drop)
				},
			},
			{
				Name:  "river-migration",
				Usage: "write the goose migration for river's schema",
				Action: func(context.Context, *cli.Command) error {
					return devdb.GenerateRiverMigration("migrations", os.Stdout)
				},
			},
			{
				Name:      "generate",
				Usage:     "write the next migration from the ent schema",
				ArgsUsage: "<migration-name>",
				Arguments: []cli.Argument{&cli.StringArg{Name: "name", Required: true}},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return devdb.GenerateMigration(ctx, "migrations", os.Getenv("DIFF_DB_URL"), cmd.StringArg("name"))
				},
			},
		},
	}
}

// onDatabase runs task on the database DATABASE_URL of this environment points at and logs what
// it did.
func onDatabase(ctx context.Context, task func(ctx context.Context, databaseURL string) (string, error)) error {
	envName := os.Getenv("SPHERICON_ENV")
	if envName == "" {
		envName = "development"
	}
	cfg, err := config.Load(envName)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	done, err := task(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	log.Print(done)
	return nil
}

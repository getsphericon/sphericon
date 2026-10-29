package main

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/rekey"
)

// rekeyOps is what `encryption-key reencrypt` needs from the app. The app implements it
// (internal/app).
type rekeyOps interface {
	Reencrypt(ctx context.Context) ([]rekey.Count, error)
}

// reencryptTimeout bounds a sweep of every stored secret.
const reencryptTimeout = 30 * time.Minute

// encryptionKeyCommand is `sphericon encryption-key rotate|reencrypt|retire`, the Self-hoster's
// way through a key rotation: `rotate` adds a primary key to the keyset in
// SPHERICON_ENCRYPTION_KEY and prints the new keyset; once every replica runs it, `reencrypt`
// seals every stored secret under that key; `retire` then prints the keyset without the old keys.
// `rotate` and `retire` print the keyset and nothing else on stdout: a script captures it.
func encryptionKeyCommand(d deps) *cli.Command {
	return group("encryption-key", "rotate SPHERICON_ENCRYPTION_KEY",
		&cli.Command{
			Name:   "rotate",
			Usage:  "add a new primary key to the keyset and print the new keyset",
			Action: keysetCommand(d, rekey.Rotate),
		},
		&cli.Command{
			Name:   "retire",
			Usage:  "drop every key but the primary from the keyset and print it",
			Action: keysetCommand(d, rekey.Retire),
		},
		&cli.Command{
			Name:  "reencrypt",
			Usage: "seal every stored secret under the primary key; idempotent",
			Action: d.withOpsWithin(reencryptTimeout, func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				counts, err := ops.Reencrypt(ctx)
				return rekey.WriteReport(out(cmd), counts, err)
			}),
		},
	)
}

// keysetCommand reads the configured keyset, changes it and prints the new keyset to stdout and
// the next step to stderr.
func keysetCommand(d deps, change func(string) (rekey.Change, error)) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
		}
		current, err := d.encryptionKey()
		if err != nil {
			return err
		}
		next, err := change(current)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out(cmd), next.Keyset)
		_, _ = fmt.Fprint(cmd.Root().ErrWriter, next.Next)
		return nil
	}
}

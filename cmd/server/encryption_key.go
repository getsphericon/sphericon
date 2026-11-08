package main

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/commandline"
)

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
			Action: keysetCommand(d, commandline.RotateKeyset),
		},
		&cli.Command{
			Name:   "retire",
			Usage:  "drop every key but the primary from the keyset and print it",
			Action: keysetCommand(d, commandline.RetireKeyset),
		},
		&cli.Command{
			Name:  "reencrypt",
			Usage: "seal every stored secret under the primary key; idempotent",
			Action: d.withCoreWithin(reencryptTimeout, func(ctx context.Context, cmd *cli.Command, c core) error {
				reencryption, err := c.SecretReencryption()
				if err != nil {
					return err
				}
				counts, err := reencryption.Run(ctx)
				return commandline.WriteReencryptionReport(out(cmd), counts, err)
			}),
		},
	)
}

// keysetCommand reads the configured keyset, changes it and prints the new keyset to stdout and
// the next step to stderr.
func keysetCommand(d deps, change func(string) (commandline.KeysetChange, error)) cli.ActionFunc {
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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/rekey"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// rekeyOps is what `encryption-key reencrypt` needs from the app. The app implements it
// (internal/app); tests use a fake.
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
			Name:  "rotate",
			Usage: "add a new primary key to the keyset and print the new keyset",
			Action: keysetCommand(d, secrets.Rotate,
				"new keyset on stdout: set it as SPHERICON_ENCRYPTION_KEY on every replica and restart; the old keys stay for decryption.\nThen run `sphericon encryption-key reencrypt`.\n"),
		},
		&cli.Command{
			Name:  "retire",
			Usage: "drop every key but the primary from the keyset and print it",
			Action: keysetCommand(d, secrets.Retire,
				"retired keyset on stdout. Run it only after `sphericon encryption-key reencrypt` reported no rewritten and no raced value, and keep the old keyset as long as you keep a backup taken before the re-encryption.\n"),
		},
		&cli.Command{
			Name:  "reencrypt",
			Usage: "seal every stored secret under the primary key; idempotent",
			Action: d.withOpsWithin(reencryptTimeout, func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				counts, err := ops.Reencrypt(ctx)
				showCounts(out(cmd), counts)
				if err != nil {
					return fmt.Errorf("re-encryption stopped: %w", err)
				}
				var rewritten, raced int
				for _, c := range counts {
					rewritten += c.Rewritten
					raced += c.Raced
				}
				if rewritten+raced > 0 {
					_, _ = fmt.Fprintln(out(cmd), "run it once more to confirm nothing is left under an older key")
				} else {
					_, _ = fmt.Fprintln(out(cmd), "every stored secret is under the primary key: the older keys can be retired (`sphericon encryption-key retire`)")
				}
				return nil
			}),
		},
	)
}

// keysetCommand reads the configured keyset, applies change and prints the result to stdout.
func keysetCommand(d deps, change func(string) (string, error), note string) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
		}
		current, err := d.encryptionKey()
		if err != nil {
			return err
		}
		if current == "" {
			return errors.New("SPHERICON_ENCRYPTION_KEY is not set: there is no keyset to change")
		}
		next, err := change(current)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out(cmd), next)
		_, _ = io.WriteString(cmd.Root().ErrWriter, note)
		return nil
	}
}

func showCounts(w io.Writer, counts []rekey.Count) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "COLUMN\tREWRITTEN\tCURRENT\tRACED")
	for _, c := range counts {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n", c.Column, c.Rewritten, c.Current, c.Raced)
	}
	_ = tw.Flush()
}

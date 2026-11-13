// Package commandline is the command line's driver package (ADR 0111): what the commands of the
// `sphericon` binary (cmd/server) print, the keyset changes they make, and the Event history they
// read and replay, which names a Workspace by the id an operator types. A command reads its
// arguments, calls one operation of a domain module or a use case, which returns a value
// (ADR 0104), and writes it here; this package is where that logic is tested, since a command has
// none.
package commandline

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// ReportSecondFactorReset reports `sphericon user reset-second-factor` of the User with email:
// whether their Second factor was cleared, or they had none (secondfactors.ErrNothingToReset).
func ReportSecondFactorReset(w io.Writer, email string, err error) error {
	if errors.Is(err, secondfactors.ErrNothingToReset) {
		_, _ = fmt.Fprintf(w, "user %q has no second factor; nothing changed\n", email)
		return nil
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "second factor of %q reset: the factor and recovery codes are cleared and every session has ended\n", email)
	return nil
}

// KeysetChange is a keyset `sphericon encryption-key rotate` or `retire` made, and what the
// Self-hoster does with it next.
type KeysetChange struct {
	Keyset string
	Next   string
}

// RotateKeyset adds a new primary key to the configured keyset; the old keys stay for decryption.
// An empty keyset is secrets.ErrNoKeyset.
func RotateKeyset(current string) (KeysetChange, error) {
	return changeKeyset(current, secrets.Rotate,
		"new keyset on stdout: set it as SPHERICON_ENCRYPTION_KEY on every replica and restart; the old keys stay for decryption.\nThen run `sphericon encryption-key reencrypt`.\n")
}

// RetireKeyset drops every key but the primary from the configured keyset. An empty keyset is
// secrets.ErrNoKeyset.
func RetireKeyset(current string) (KeysetChange, error) {
	return changeKeyset(current, secrets.Retire,
		"retired keyset on stdout. Run it only after `sphericon encryption-key reencrypt` reported no rewritten and no raced value, and keep the old keyset as long as you keep a backup taken before the re-encryption.\n")
}

func changeKeyset(current string, apply func(string) (string, error), next string) (KeysetChange, error) {
	keyset, err := apply(current)
	if err != nil {
		return KeysetChange{}, err
	}
	return KeysetChange{Keyset: keyset, Next: next}, nil
}

// WriteReencryptionReport prints what a re-encryption did, one line per column, then the next
// step: run it once more while it rewrote or skipped a value, retire the older keys once it found
// every value current. A run that stopped (runErr) shows what it did before it stopped and
// returns the error.
func WriteReencryptionReport(w io.Writer, counts []secrets.Count, runErr error) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "COLUMN\tREWRITTEN\tCURRENT\tRACED")
	moved := 0
	for _, c := range counts {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n", c.Column, c.Rewritten, c.Current, c.Raced)
		moved += c.Rewritten + c.Raced
	}
	_ = tw.Flush()
	if runErr != nil {
		return fmt.Errorf("re-encryption stopped: %w", runErr)
	}
	if moved > 0 {
		_, _ = fmt.Fprintln(w, "run it once more to confirm nothing is left under an older key")
		return nil
	}
	_, _ = fmt.Fprintln(w, "every stored secret is under the primary key: the older keys can be retired (`sphericon encryption-key retire`)")
	return nil
}

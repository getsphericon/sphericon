package rekey

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/getsphericon/sphericon/internal/secrets"
)

// ErrNoKeyset means SPHERICON_ENCRYPTION_KEY is empty, so there is no keyset to rotate or retire.
var ErrNoKeyset = errors.New("SPHERICON_ENCRYPTION_KEY is not set: there is no keyset to change")

// Change is a keyset `sphericon encryption-key rotate` or `retire` made, and what the
// Self-hoster does with it next.
type Change struct {
	Keyset string
	Next   string
}

// Rotate adds a new primary key to the configured keyset; the old keys stay for decryption.
func Rotate(current string) (Change, error) {
	return change(current, secrets.Rotate,
		"new keyset on stdout: set it as SPHERICON_ENCRYPTION_KEY on every replica and restart; the old keys stay for decryption.\nThen run `sphericon encryption-key reencrypt`.\n")
}

// Retire drops every key but the primary from the configured keyset.
func Retire(current string) (Change, error) {
	return change(current, secrets.Retire,
		"retired keyset on stdout. Run it only after `sphericon encryption-key reencrypt` reported no rewritten and no raced value, and keep the old keyset as long as you keep a backup taken before the re-encryption.\n")
}

func change(current string, apply func(string) (string, error), next string) (Change, error) {
	if current == "" {
		return Change{}, ErrNoKeyset
	}
	keyset, err := apply(current)
	if err != nil {
		return Change{}, err
	}
	return Change{Keyset: keyset, Next: next}, nil
}

// WriteReport prints what a re-encryption did, one line per column, then the next step: run it
// once more while it rewrote or skipped a value, retire the older keys once it found every
// value current. A run that stopped (runErr) shows what it did before it stopped and returns
// the error.
func WriteReport(w io.Writer, counts []Count, runErr error) error {
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

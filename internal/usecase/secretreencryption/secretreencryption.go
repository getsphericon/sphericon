// Package secretreencryption is the use case behind `sphericon encryption-key reencrypt`
// (ADR 0102): it stores every secret in the database again under the primary key of the
// Encryption key, so a key an operator rotated away from can be retired. The secrets span every
// Workspace (and the User and Staff member rows no Workspace owns), and each table belongs to a
// domain module: each owner hands the wiring the columns it holds as secrets.Sweep values
// (SecretSweeps), which read and compare-and-swap on the raw client, and the use case sweeps them
// in the order given (secrets.Cipher.Reencrypt) and holds no client itself. The package also holds
// the keyset changes of `encryption-key rotate` and `retire` and the report of a run.
package secretreencryption

import (
	"context"

	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// UseCase re-encrypts the stored secrets.
type UseCase struct {
	sweeps []secrets.Sweep
	cipher *secrets.Cipher
}

// New builds the use case on the owners' columns of stored secrets, in the order they are swept,
// and the cipher whose primary key is the target.
func New(sweeps []secrets.Sweep, cipher *secrets.Cipher) *UseCase {
	return &UseCase{sweeps: sweeps, cipher: cipher}
}

// Run sweeps every column that holds ciphertext, in order, and returns what it did per column. It
// fails at the first value the cipher cannot read, which means a key is missing from the keyset:
// nothing guesses, what was already rewritten stays valid, and the counts are those of the
// columns it finished.
func (u *UseCase) Run(ctx context.Context) ([]secrets.Count, error) {
	return u.cipher.Reencrypt(ctx, u.sweeps...)
}

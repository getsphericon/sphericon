package secrets

import (
	"context"
	"fmt"
)

// sweepPageSize is how many rows a sweep loads at once.
const sweepPageSize = 200

// Count is what one sweep did to a column of stored secrets.
type Count struct {
	// Column names the stored secret, as "table.column".
	Column string
	// Rewritten values were sealed under an older key and now are under the primary one.
	Rewritten int
	// Current values were already under the primary key and were left alone.
	Current int
	// Raced rows changed while the sweep ran (a request saved them): their new value was
	// sealed by the running server, so they are skipped. A second run counts them as current.
	Raced int
}

// Stored is one stored value of a sweep: the row id and what it holds.
type Stored struct {
	ID    int64
	Value string
}

// Sweep is one column of stored secrets, as the module that owns its table reads and writes it.
type Sweep struct {
	// Column names it, as "table.column".
	Column string
	// Page returns up to n rows with an id above after, in id order.
	Page func(ctx context.Context, after int64, n int) ([]Stored, error)
	// Swap compares and swaps: it stores next on the row only while it still holds prev, and
	// reports whether it did.
	Swap func(ctx context.Context, id int64, prev, next string) (bool, error)
	// Seal maps a stored value to the one to store under the primary key of the cipher; nil
	// rewraps it whole (Cipher.Rewrap).
	Seal func(c *Cipher, value string) (string, bool, error)
}

// Reencrypt sweeps the columns in order, each in id order and page by page, and stores every
// value sealed under an older key again under the primary key. It returns the count of every
// column it finished. It stops at the first value the cipher cannot read, which means a key is
// missing from the keyset, with the column and the row in the error: nothing guesses, and what
// was already rewritten stays valid.
func (c *Cipher) Reencrypt(ctx context.Context, sweeps ...Sweep) ([]Count, error) {
	counts := make([]Count, 0, len(sweeps))
	for _, s := range sweeps {
		n, err := c.sweep(ctx, s)
		if err != nil {
			return counts, fmt.Errorf("%s: %w", s.Column, err)
		}
		counts = append(counts, n)
	}
	return counts, nil
}

func (c *Cipher) sweep(ctx context.Context, s Sweep) (Count, error) {
	n := Count{Column: s.Column}
	seal := s.Seal
	if seal == nil {
		seal = (*Cipher).Rewrap
	}
	var after int64
	for {
		rows, err := s.Page(ctx, after, sweepPageSize)
		if err != nil {
			return n, err
		}
		for _, r := range rows {
			after = r.ID
			if r.Value == "" {
				continue
			}
			next, changed, err := seal(c, r.Value)
			if err != nil {
				return n, fmt.Errorf("row %d: %w", r.ID, err)
			}
			if !changed {
				n.Current++
				continue
			}
			swapped, err := s.Swap(ctx, r.ID, r.Value, next)
			if err != nil {
				return n, fmt.Errorf("row %d: %w", r.ID, err)
			}
			if swapped {
				n.Rewritten++
			} else {
				n.Raced++
			}
		}
		if len(rows) < sweepPageSize {
			return n, nil
		}
	}
}

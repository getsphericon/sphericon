package secrets

import (
	"context"
	"errors"
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
	// Raced rows changed while the sweep ran (a request saved them), or were held by an edit, so
	// they are skipped: a second run rewrites them or counts them as current.
	Raced int
}

// Stored is one stored value of a sweep: the row id and what it holds.
type Stored struct {
	ID    int64
	Value string
}

// Sweep is one column of stored secrets, as the module that owns its table reads and writes it:
// by a compare-and-swap of the text it holds (Page, Swap, Seal), or, for a document whose writes
// its owner's revision guards, row by row (Page, Rewrite).
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
	// Rewrite, set in place of Swap and Seal, rewrites the row of each id Page lists on its own
	// (Guarded.Rewrite), and reports what it did. Page then lists the ids alone.
	Rewrite func(ctx context.Context, c *Cipher, id int64) (Outcome, error)
}

// Outcome is what a sweep did to one row.
type Outcome int

// The outcomes of a row, each counted in its field of Count.
const (
	// RowCurrent is a row already under the primary key, left alone.
	RowCurrent Outcome = iota
	// RowRewritten is a row sealed again under the primary key.
	RowRewritten
	// RowRaced is a row edited, or held by an edit, each time it was about to be written: left to
	// the next run.
	RowRaced
)

// ErrRowGone is what a Guarded Read returns for a row deleted since its page was listed (a draft
// dropped by a Revert): it holds no secret any more, so it counts as current.
var ErrRowGone = errors.New("secrets: the row is gone")

// maxRaces is how many times a guarded rewrite reads a row again after an edit took it, before it
// leaves the row to the next run.
const maxRaces = 5

// Guarded is one row of a document whose every write moves its owner's revision (ADR 0119), as a
// Rule's or an Automation's moves with an edit of its versions (ADR 0169). Read returns the
// document and the owner's revision read no later than it; Seal maps the document to the one to
// store under the primary key, and reports whether it changed it; Read returns ErrRowGone for a
// row deleted meanwhile; Write stores a document only
// while the owner still has the revision given, and reports whether it did.
type Guarded[T any] struct {
	Read  func(ctx context.Context) (doc T, revision int64, err error)
	Seal  func(c *Cipher, doc T) (T, bool, error)
	Write func(ctx context.Context, doc T, revision int64) (bool, error)
}

// Rewrite reads the row, seals its document again and writes it on the revision it read. A write
// that misses lost to an edit, which wrote its own document: it reads the row again and seals that
// one, so the edit stays. A row raced maxRaces times is RowRaced.
func (g Guarded[T]) Rewrite(ctx context.Context, c *Cipher) (Outcome, error) {
	for range maxRaces {
		doc, revision, err := g.Read(ctx)
		if errors.Is(err, ErrRowGone) {
			return RowCurrent, nil
		}
		if err != nil {
			return RowCurrent, err
		}
		next, changed, err := g.Seal(c, doc)
		if err != nil || !changed {
			return RowCurrent, err
		}
		wrote, err := g.Write(ctx, next, revision)
		if err != nil {
			return RowCurrent, err
		}
		if wrote {
			return RowRewritten, nil
		}
	}
	return RowRaced, nil
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
			if s.Rewrite != nil {
				outcome, err := s.Rewrite(ctx, c, r.ID)
				if err != nil {
					return n, fmt.Errorf("row %d: %w", r.ID, err)
				}
				n.add(outcome)
				continue
			}
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

// add counts one row's outcome.
func (n *Count) add(o Outcome) {
	switch o {
	case RowRewritten:
		n.Rewritten++
	case RowRaced:
		n.Raced++
	default:
		n.Current++
	}
}

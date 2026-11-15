package signup

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// SetLookup replaces the lookup of the User that holds an address, so that the race test of two
// Sign-ups of one new address replays a lookup that missed the winner's User (step 3 of SignUp).
// It is the one export_test.go review rule 11 allows.
func (m *Module) SetLookup(lookup func(ctx context.Context, email string) (*ent.User, bool, error)) {
	m.lookup = lookup
}

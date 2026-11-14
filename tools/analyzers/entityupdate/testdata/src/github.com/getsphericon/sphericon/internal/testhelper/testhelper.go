package testhelper

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The test harness is read as well: no package of the module is exempt.
func Touch(ctx context.Context, t *ent.Tag) error {
	return t.Update().Exec(ctx) // want `^rule 0: Update on a loaded Tag`
}

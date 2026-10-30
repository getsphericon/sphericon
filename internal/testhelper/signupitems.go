package testhelper

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/signupitems"
)

// PutSignupItemsInEffect brings the fixture Sign-up items with these ids back from the archive: the
// fixture Instance asks nothing at sign-up, so a test that wants items asked lifts some of them
// instead of creating items.
func (env *TestEnv) PutSignupItemsInEffect(ctx context.Context, ids ...int64) {
	if err := env.DB.SignupItem.Update().Where(signupitem.IDIn(ids...)).ClearDiscardedAt().Exec(ctx); err != nil {
		panic(err)
	}
}

// ConsentRecords is the Consent records module over a client of a test's own database (one from
// RealDB), for a test that builds a module needing it outside Setup.
func ConsentRecords(client *ent.Client, bus *events.Bus) *consentrecords.Module {
	return consentrecords.New(client, bus, signupitems.New(client, bus, nil))
}

package identify_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/identify"
)

func TestAContactCreatedByIdentifyCarriesTheTrackerSource(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := identify.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), identify.Input{VisitorID: "dev-source", Email: new("source-tracker@example.com")})
	require.NoError(t, err)

	created, err := env.DB.Contact.Query().Where(contact.Email("source-tracker@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, contact.SourceTracker, lo.FromPtr(created.Source))
}

func TestIdentifyOfAnExistingContactLeavesItsSource(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := identify.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), identify.Input{VisitorID: "dev-alice", Email: new(fixtures.ContactAliceEmail)})
	require.NoError(t, err)

	alice, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Nil(t, alice.Source, "a Contact that predates the field stays unknown")
}

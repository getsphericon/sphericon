package visitoridentification_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/visitoridentification"
)

func TestAContactCreatedByIdentifyCarriesTheTrackerSource(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{VisitorID: "dev-source", Email: new("source-tracker@example.com")})
	require.NoError(t, err)

	created, err := env.DB.Contact.Query().Where(contact.Email("source-tracker@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, contact.SourceTracker, created.Source)
}

func TestIdentifyOfAnExistingContactLeavesItsSource(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{VisitorID: "dev-alice", Email: new(fixtures.ContactAliceEmail)})
	require.NoError(t, err)

	alice, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Equal(t, contact.SourceManual, alice.Source, "Identify keeps the source the Contact was created with")
}

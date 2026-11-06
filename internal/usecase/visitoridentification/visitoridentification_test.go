package visitoridentification_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/visitoridentification"
)

func TestIdentifyCreatesTheContactBindsTheDeviceAndStitchesEarlierEvents(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The fixtures hold an earlier anonymous event from the device, and one from another device.
	const device, otherDevice = fixtures.EventAnonymousFirstVisitorID, fixtures.EventAnonymousSecondVisitorID

	err := visitoridentification.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
		VisitorID: "  " + device + " ",
		Email:     new("Visitor@Example.com"),
		Traits:    map[string]any{"plan": "pro"},
	})
	require.NoError(t, err)

	c, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email("visitor@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "pro", c.CustomFields["plan"])

	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID(device)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, c.ID, lo.FromPtr(v.ContactID), "the device is bound to the contact")

	stitched, err := env.DB.Event.Query().Where(event.VisitorID(device)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, c.ID, lo.FromPtr(stitched.ContactID), "pre-identify behavior is attached")
	other, err := env.DB.Event.Query().Where(event.VisitorID(otherDevice)).Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, other.ContactID, "another device stays anonymous")
}

func TestIdentifyReusesAnExistingContactAndDevice(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	input := visitoridentification.Input{VisitorID: "dev-known", Email: lo.ToPtr(fixtures.ContactAliceEmail)}

	require.NoError(t, visitoridentification.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), input))
	require.NoError(t, visitoridentification.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), input), "identify is idempotent")

	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-known")).Only(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.ContactAliceID, lo.FromPtr(v.ContactID))
	n, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email(fixtures.ContactAliceEmail)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "no duplicate contact")
}

func TestIdentifyRefusesWhatCannotBeIdentified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := visitoridentification.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{VisitorID: "   ", Email: new("a@example.com")})
	require.EqualError(t, err, "visitorId is required")

	err = visitoridentification.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{VisitorID: "dev-x"})
	require.EqualError(t, err, "identify requires subjectId, email, or phone")
	n, err := env.DB.Visitor.Query().Where(visitor.VisitorID("dev-x")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a refused identify leaves no device behind")
}

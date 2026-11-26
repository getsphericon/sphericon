package visitoridentification_test

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/identitytokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/visitoridentification"
)

func TestIdentifyCreatesTheContactBindsTheDeviceAndStitchesEarlierEvents(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	// The fixtures hold an earlier anonymous event from the device, and one from another device.
	const device, otherDevice = fixtures.EventAnonymousFirstVisitorID, fixtures.EventAnonymousSecondVisitorID

	err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
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
	ctx := t.Context()
	input := visitoridentification.Input{VisitorID: "dev-known", Email: lo.ToPtr(fixtures.ContactAliceEmail)}

	require.NoError(t, testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), input))
	require.NoError(t, testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), input), "identify is idempotent")

	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-known")).Only(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.ContactAliceID, lo.FromPtr(v.ContactID))
	n, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email(fixtures.ContactAliceEmail)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "no duplicate contact")
}

// An Identify that changes nothing writes nothing: the Contact keeps its revision and updated_at,
// so a console editor open on it can still save (ADR 0119). A claim never replaces a value
// (ADR 0171), so another plan changes nothing either; one that fills an empty attribute moves both.
func TestIdentifyThatChangesNothingLeavesTheContactAsItWas(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	identify := func(plan string) {
		t.Helper()
		err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
			VisitorID: fixtures.EventAnonymousFirstVisitorID,
			Email:     new("repeat@example.com"),
			Traits:    map[string]any{"plan": plan, "firstName": "Rita"},
		})
		require.NoError(t, err)
	}
	stored := func() (int64, time.Time) {
		c, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email("repeat@example.com")).Only(ctx)
		require.NoError(t, err)
		return c.Revision, c.UpdatedAt
	}
	identify("pro")
	revision, updatedAt := stored()

	identify("pro")
	again, againAt := stored()
	assert.Equal(t, revision, again)
	assert.True(t, updatedAt.Equal(againAt), "updated_at %v, then %v", updatedAt, againAt)

	identify("free")
	unchanged, _ := stored()
	assert.Equal(t, revision, unchanged, "a claim never replaces")

	err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
		VisitorID: fixtures.EventAnonymousFirstVisitorID,
		Email:     new("repeat@example.com"),
		Traits:    map[string]any{"lastName": "Levi"},
	})
	require.NoError(t, err)
	changed, _ := stored()
	assert.Equal(t, revision+1, changed, "a claim fills what is empty")
}

// A claim binds only a Visitor that is anonymous or already the Contact's (ADR 0171): a Visitor of
// another Contact stays where it is, and the Contact the claim names is still found or created.
func TestAClaimLeavesAVisitorOfAnotherContactWhereItIs(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	identify := func(email string) {
		t.Helper()
		err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
			VisitorID: "dev-claimed", Email: new(email),
		})
		require.NoError(t, err)
	}

	identify(fixtures.ContactAliceEmail)
	identify("someone.else@example.com")

	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-claimed")).Only(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.ContactAliceID, lo.FromPtr(v.ContactID), "the Visitor stays Alice's")
	n, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email("someone.else@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the claimed Contact is created")
}

// A claim gives a Contact that exists no alias key (ADR 0171): Alice has no phone, and a claim
// naming her email and a phone leaves her without one.
func TestAClaimFillsNoAliasKeyOfAContactThatExists(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
		VisitorID: "dev-phone", Email: lo.ToPtr(fixtures.ContactAliceEmail), Phone: new("+14155550199"), SubjectID: new("claimed-subject"),
	})
	require.NoError(t, err)

	alice, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	assert.Nil(t, alice.Phone)
	assert.Nil(t, alice.SubjectID)
}

// Traits of every type read back from the stored JSON as another Go type than they are given in (a
// number as float64, a moment as text): repeating them is still no change, so the Contact keeps its
// revision.
func TestIdentifyRepeatingTraitsOfEveryTypeLeavesTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	traits := map[string]any{
		"seats":    4,
		"ratio":    json.Number("2.5"),
		"beta":     true,
		"renewsAt": "2026-03-01T10:00:00Z",
		"address":  map[string]any{"city": "Lisbon", "floors": []any{1, 2}},
	}
	identify := func() int64 {
		t.Helper()
		err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
			VisitorID: fixtures.EventAnonymousFirstVisitorID,
			Email:     new("typed@example.com"),
			Traits:    traits,
		})
		require.NoError(t, err)
		c, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email("typed@example.com")).Only(ctx)
		require.NoError(t, err)
		return c.Revision
	}

	_, err := testhelper.Module[*contacts.Module](env).EnsureCustomFields(ctx, env.DB.Scoped(fixtures.AcmeID), traits)
	require.NoError(t, err, "a claim writes only defined Custom fields")
	first := identify()

	assert.Equal(t, first, identify())
}

func TestIdentifyRefusesWhatCannotBeIdentified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{VisitorID: "   ", Email: new("a@example.com")})
	require.ErrorIs(t, err, visitoridentification.ErrVisitorIDRequired)

	err = testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{VisitorID: "dev-x"})
	require.ErrorIs(t, err, contacts.ErrIdentityRequired)
	n, err := env.DB.Visitor.Query().Where(visitor.VisitorID("dev-x")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a refused identify leaves no device behind")
}

// An Identify whose keys or browser guesses are longer than the contract allows is refused, every
// such field named, and nothing is created.
func TestIdentifyRefusesFieldsLongerThanTheContract(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	err := testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
		VisitorID: "dev-overlong",
		SubjectID: new(strings.Repeat("s", 256)),
		Phone:     new(strings.Repeat("1", 33)),
		TimeZone:  new(strings.Repeat("z", 65)),
		Locale:    new(strings.Repeat("l", 36)),
	})

	form, ok := errors.AsType[*validation.FieldsError](err)
	require.True(t, ok, "a form refusal: %v", err)
	assert.ElementsMatch(t, []string{"subjectId", "phone", "timeZone", "locale"}, slices.Collect(maps.Keys(form.Fields)))
	seen, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-overlong")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, seen, "the device is not recorded")
}

// A proven Identify writes the token's keys, so they meet the contract's bounds as a sent key does.
func TestAProvenIdentifyRefusesATokenWhoseKeyIsLongerThanTheContract(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	secret, err := testhelper.Module[*identitytokens.Module](env).Regenerate(ctx, acme)
	require.NoError(t, err)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": strings.Repeat("s", 256), "exp": time.Now().Add(time.Minute).Unix(),
	}).SignedString([]byte(secret))
	require.NoError(t, err)

	err = testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, acme, visitoridentification.Input{
		VisitorID: "dev-long-token", IdentityToken: token,
	})

	form, ok := errors.AsType[*validation.FieldsError](err)
	require.True(t, ok, "a form refusal: %v", err)
	assert.Contains(t, form.Fields, "subjectId")
}

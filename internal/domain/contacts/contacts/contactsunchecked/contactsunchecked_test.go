package contactsunchecked_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts/contactsunchecked"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// update opens the edit in of Acme's Custom field id under Acme's lock and runs then on it, in
// the transaction that holds the lock.
func update(t *testing.T, env *testhelper.TestEnv, id int64, in contacts.CustomFieldUpdateInput, then func(ctx context.Context, u *contactsunchecked.CustomFieldUpdate) error) error {
	t.Helper()
	ctx := t.Context()
	return env.Bus.WithinWorkspaceLockTx(ctx, env.DB.Scoped(fixtures.AcmeID), func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		u, err := contactsunchecked.CustomFieldUpdateIn(ctx, wl, id, in)
		if err != nil {
			return err
		}
		return then(ctx, u)
	})
}

// plan reads Acme's Custom field plan as it is now.
func plan(t *testing.T, env *testhelper.TestEnv) *ent.CustomField {
	t.Helper()
	field, err := contacts.CustomField(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.CustomFieldAcmePlanID)
	require.NoError(t, err)
	return field
}

func TestApplyingARetypeChangesTheTypeOfTheField(t *testing.T) {
	env := testhelper.Setup(t)
	in := contacts.CustomFieldUpdateInput{Revision: plan(t, env).Revision, Type: patch.Set(string(customfield.TypeNumber))}
	var key string
	var retype customfield.Type
	var retypes bool

	err := update(t, env, fixtures.CustomFieldAcmePlanID, in, func(ctx context.Context, u *contactsunchecked.CustomFieldUpdate) error {
		key = u.Key()
		retype, retypes = u.Retype()
		_, err := u.Apply(ctx)
		return err
	})

	require.NoError(t, err)
	assert.Equal(t, fixtures.CustomFieldAcmePlanKey, key, "the Removal check reads the key the edit opened")
	assert.True(t, retypes)
	assert.Equal(t, customfield.TypeNumber, retype)
	edited := plan(t, env)
	assert.Equal(t, customfield.TypeNumber, edited.Type)
	assert.Equal(t, fixtures.CustomFieldAcmePlanName, edited.Name)
}

// An edit that names the type the field has already, or none, is no retype: the Removal check
// has nothing to ask.
func TestAnEditThatKeepsTheTypeIsNoRetype(t *testing.T) {
	env := testhelper.Setup(t)
	revision := plan(t, env).Revision
	for name, in := range map[string]contacts.CustomFieldUpdateInput{
		"the same type": {Revision: revision, Type: patch.Set(string(customfield.TypeString))},
		"a rename":      {Revision: revision, Name: patch.Set("Pricing plan")},
	} {
		t.Run(name, func(t *testing.T) {
			var retypes bool

			err := update(t, env, fixtures.CustomFieldAcmePlanID, in, func(_ context.Context, u *contactsunchecked.CustomFieldUpdate) error {
				_, retypes = u.Retype()
				return nil
			})

			require.NoError(t, err)
			assert.False(t, retypes)
		})
	}
}

// The edit is opened on a revision (ADR 0119) and with a name the name rule takes, or it is
// refused before the use case's check runs, and nothing is written.
func TestOpeningAnEditRefusesWhatTheFieldsOwnRulesRefuse(t *testing.T) {
	env := testhelper.Setup(t)
	before := plan(t, env)
	for _, tc := range []struct {
		name    string
		in      contacts.CustomFieldUpdateInput
		refusal error
	}{
		{"a revision the field has moved past", contacts.CustomFieldUpdateInput{Revision: before.Revision - 1, Name: patch.Set("Pricing plan")}, contacts.ErrCustomFieldRevisionChanged},
		{"a blank name", contacts.CustomFieldUpdateInput{Revision: before.Revision, Name: patch.Set("  ")}, validation.ErrNameBlank},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened := false

			err := update(t, env, fixtures.CustomFieldAcmePlanID, tc.in, func(context.Context, *contactsunchecked.CustomFieldUpdate) error {
				opened = true
				return nil
			})

			require.ErrorIs(t, err, tc.refusal)
			assert.False(t, opened)
			after := plan(t, env)
			assert.Equal(t, before.Name, after.Name)
			assert.Equal(t, before.Revision, after.Revision)
		})
	}
}

func TestOpeningAnEditOfAnUnknownFieldIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	err := update(t, env, 999999, contacts.CustomFieldUpdateInput{Revision: 1}, func(context.Context, *contactsunchecked.CustomFieldUpdate) error {
		return nil
	})

	require.ErrorIs(t, err, contacts.ErrCustomFieldNotFound)
}

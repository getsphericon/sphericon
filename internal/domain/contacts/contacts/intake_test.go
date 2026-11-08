package contacts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestUpsertBatchRunsEachItemIndependentlyAndInOrder(t *testing.T) {
	env := testhelper.Setup(t)
	intake := contacts.New(env.Bus)

	out := intake.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, []contacts.IntakeInput{
		{Email: new("batch.new@example.com"), CustomFields: map[string]any{"source": "import"}},
		{Email: lo.ToPtr(fixtures.ContactAliceEmail), FirstName: patch.Set("Alicia")},
		{FirstName: patch.Set("No identity")},
	})
	require.Len(t, out.Items, 3)

	require.NoError(t, out.Items[0].Err)
	assert.True(t, out.Items[0].Value.Created)
	assert.Equal(t, "import", out.Items[0].Value.Contact.CustomFields["source"])
	assert.Equal(t, 1, createdEvents(t, env, out.Items[0].Value.Contact.ID))

	require.NoError(t, out.Items[1].Err)
	assert.False(t, out.Items[1].Value.Created)
	assert.EqualValues(t, fixtures.ContactAliceID, out.Items[1].Value.Contact.ID)
	assert.Equal(t, "Alicia", lo.FromPtr(out.Items[1].Value.Contact.FirstName), "an asserted attribute overwrites")

	require.ErrorIs(t, out.Items[2].Err, contacts.ErrIdentityRequired, "a failing item does not stop the batch")
}

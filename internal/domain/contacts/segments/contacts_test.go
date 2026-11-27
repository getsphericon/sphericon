package segments_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// idsOf is the ids of Contacts, in their order.
func idsOf(cs []*ent.Contact) []int64 {
	return lo.Map(cs, func(c *ent.Contact, _ int) int64 { return c.ID })
}

// makePro makes a Contact of Acme a member of the Pro plan Segment.
func makePro(t *testing.T, env *testhelper.TestEnv, id int64) {
	t.Helper()
	err := env.DB.Scoped(fixtures.AcmeID).Contact().UpdateOneID(id).
		SetCustomFields(map[string]any{"plan": "pro"}).Exec(t.Context())
	require.NoError(t, err)
}

// A Segment's Contacts are read by a keyset cursor on the Contact id: each member comes once
// across the pages while members are added, one before the cursor (which offset paging would shift
// into a second read) and one after it (read when the cursor reaches it).
func TestContactsReadsEachMemberOnceWhileMembersAreAdded(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ws := env.DB.Scoped(fixtures.AcmeID)
	segmentID := int64(fixtures.SegmentProPlanID)
	before, err := m.Contacts(t.Context(), ws, segments.ContactsInput{SegmentID: &segmentID, Limit: 1000})
	require.NoError(t, err)
	members := idsOf(before.Items)
	require.Greater(t, len(members), 2)
	all, err := ws.Contact().Query().Order(ent.Asc(contact.FieldID)).IDs(t.Context())
	require.NoError(t, err)
	outsiders := lo.Without(all, members...)
	earlier, found := lo.Find(outsiders, func(id int64) bool { return id < members[1] })
	require.True(t, found)
	later, found := lo.Find(outsiders, func(id int64) bool { return id > members[1] })
	require.True(t, found)

	first, err := m.Contacts(t.Context(), ws, segments.ContactsInput{SegmentID: &segmentID, Limit: 2})
	require.NoError(t, err)
	makePro(t, env, earlier)
	makePro(t, env, later)
	read := idsOf(first.Items)
	next := first.Next
	for next != 0 {
		page, err := m.Contacts(t.Context(), ws, segments.ContactsInput{SegmentID: &segmentID, After: next, Limit: 2})
		require.NoError(t, err)
		read = append(read, idsOf(page.Items)...)
		next = page.Next
	}

	assert.ElementsMatch(t, append(members, later), read)
	assert.IsIncreasing(t, read)
}

// Without a Segment the cursor reads every Contact of the Workspace.
func TestContactsWithoutASegmentReadsEveryContact(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ws := env.DB.Scoped(fixtures.TyrellID)
	all, err := ws.Contact().Query().Order(ent.Asc(contact.FieldID)).IDs(t.Context())
	require.NoError(t, err)

	first, err := m.Contacts(t.Context(), ws, segments.ContactsInput{Limit: 3})
	require.NoError(t, err)
	rest, err := m.Contacts(t.Context(), ws, segments.ContactsInput{After: first.Next, Limit: len(all)})
	require.NoError(t, err)

	assert.Equal(t, all, append(idsOf(first.Items), idsOf(rest.Items)...))
	assert.Zero(t, rest.Next, "the last page has no next")
}

// An unknown Segment is ErrNotFound.
func TestContactsOfAnUnknownSegmentIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	missing := int64(999999)

	_, err := m.Contacts(t.Context(), env.DB.Scoped(fixtures.AcmeID), segments.ContactsInput{SegmentID: &missing, Limit: 10})

	require.ErrorIs(t, err, segments.ErrNotFound)
}

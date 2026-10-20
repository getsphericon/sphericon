package staff_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entmembership "github.com/getsphericon/sphericon/ent/membership"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// feedPage reads one page of the Workspace feed from cursor.
func feedPage(t *testing.T, c *serviceapi.Client, cursor string, limit int32) *serviceapi.ServiceWorkspaceFeedPage {
	t.Helper()
	params := serviceapi.ServiceWorkspaceFeedListParams{}
	if cursor != "" {
		params.Cursor = serviceapi.NewOptString(cursor)
	}
	if limit > 0 {
		params.Limit = serviceapi.NewOptInt32(limit)
	}
	res, err := c.ServiceWorkspaceFeedList(t.Context(), params)
	require.NoError(t, err)
	page, ok := res.(*serviceapi.ServiceWorkspaceFeedPage)
	require.Truef(t, ok, "the feed is answered, got %T", res)
	return page
}

// feedFrom reads every page from cursor and returns the entries and the cursor to save.
func feedFrom(t *testing.T, c *serviceapi.Client, cursor string) ([]serviceapi.ServiceWorkspaceResource, string) {
	t.Helper()
	var all []serviceapi.ServiceWorkspaceResource
	for {
		page := feedPage(t, c, cursor, 2)
		all = append(all, page.Items...)
		cursor = page.NextCursor
		if !page.HasMore {
			return all, cursor
		}
	}
}

func feedIDs(entries []serviceapi.ServiceWorkspaceResource) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = string(e.ID)
	}
	return out
}

func ownerIDs(w serviceapi.ServiceWorkspaceResource) []int64 {
	out := make([]int64, len(w.Owners))
	for i, o := range w.Owners {
		out[i] = int64(o.UserId)
	}
	return out
}

func entry(t *testing.T, entries []serviceapi.ServiceWorkspaceResource, id string) serviceapi.ServiceWorkspaceResource {
	t.Helper()
	for _, e := range entries {
		if string(e.ID) == id {
			return e
		}
	}
	require.FailNowf(t, "entry missing", "no %q in %v", id, feedIDs(entries))
	return serviceapi.ServiceWorkspaceResource{}
}

func TestTheFeedFromAnEmptyCursorListsEveryWorkspaceWithAllOfItsOwners(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)

	entries, _ := feedFrom(t, c, "")

	all, err := h.env.DB.Workspace.Query().All(t.Context())
	require.NoError(t, err)
	require.Greater(t, len(all), 3)
	want := make([]string, len(all))
	for i, w := range all {
		want[i] = w.PublicID
	}
	assert.ElementsMatch(t, want, feedIDs(entries), "every Workspace is in the feed at least once")
	initech := entry(t, entries, fixtures.InitechPublicID)
	assert.ElementsMatch(t, []int64{5, 6}, ownerIDs(initech), "a co-owned Workspace carries both owners")
	for _, o := range initech.Owners {
		switch o.UserId {
		case 5:
			assert.Equal(t, fixtures.InitechOwnerOlgaEmail, string(o.Email))
		case 6:
			assert.Equal(t, fixtures.InitechCoOwnerOwenEmail, string(o.Email))
		}
	}
	assert.Equal(t, []int64{1}, ownerIDs(entry(t, entries, fixtures.AcmePublicID)), "only owners, never members")
}

func TestReplayingACursorReturnsTheSameEntriesAndTheCursorOnlyMovesWhenSaved(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	first := feedPage(t, c, "", 2)
	require.Len(t, first.Items, 2)
	require.True(t, first.HasMore)

	again := feedPage(t, c, "", 2)
	assert.Equal(t, first, again, "a page read twice from the same cursor is the same page")

	rest := feedPage(t, c, first.NextCursor, 10)
	assert.False(t, rest.HasMore)
	assert.NotEmpty(t, rest.Items)
	assert.Equal(t, rest, feedPage(t, c, first.NextCursor, 10), "and so is the page after a saved cursor")

	end := rest.NextCursor
	empty := feedPage(t, c, end, 10)
	assert.Empty(t, empty.Items, "nothing changed since the saved cursor")
	assert.Equal(t, end, empty.NextCursor, "an empty page keeps the cursor")
}

func TestANewWorkspaceEntersTheFeedWithItsOwner(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	_, saved := feedFrom(t, c, "")

	ws, err := accounts.New(h.env.DB, h.env.Bus).CreateWorkspace(t.Context(), fixtures.MemberMaryID, "Hooli")
	require.NoError(t, err)

	got, _ := feedFrom(t, c, saved)
	require.Equal(t, []string{ws.PublicID}, feedIDs(got))
	assert.Equal(t, []int64{3}, ownerIDs(got[0]))
	assert.Equal(t, fixtures.MemberMaryEmail, string(got[0].Owners[0].Email))
}

func TestAnOwnerAddedRemovedOrTransferredPutsTheWorkspaceBackInTheFeed(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	initech := h.env.DB.Scoped(fixtures.InitechID)
	_, saved := feedFrom(t, c, "")

	// Added.
	added, err := initech.Membership().Create().SetUserID(fixtures.MemberMaryID).SetRole(entmembership.RoleOwner).Save(t.Context())
	require.NoError(t, err)
	got, saved := feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.InitechPublicID}, feedIDs(got))
	assert.ElementsMatch(t, []int64{3, 5, 6}, ownerIDs(got[0]))

	// Transferred: the co-owner steps down.
	_, err = initech.Membership().UpdateOneID(fixtures.InitechCoOwnerMembershipID).SetRole(entmembership.RoleAdmin).Save(t.Context())
	require.NoError(t, err)
	got, saved = feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.InitechPublicID}, feedIDs(got))
	assert.ElementsMatch(t, []int64{3, 5}, ownerIDs(got[0]))

	// Removed.
	require.NoError(t, initech.Membership().DeleteOneID(added.ID).Exec(t.Context()))
	got, _ = feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.InitechPublicID}, feedIDs(got))
	assert.Equal(t, []int64{5}, ownerIDs(got[0]))
}

func TestAMemberJoiningOrLeavingIsNotAChangeOfOwners(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	initech := h.env.DB.Scoped(fixtures.InitechID)
	_, saved := feedFrom(t, c, "")

	m, err := initech.Membership().Create().SetUserID(fixtures.MemberMaryID).SetRole(entmembership.RoleMember).Save(t.Context())
	require.NoError(t, err)
	require.NoError(t, initech.Membership().DeleteOneID(m.ID).Exec(t.Context()))

	got, _ := feedFrom(t, c, saved)
	assert.Empty(t, got)
}

func TestAnOwnersEmailOrVerificationChangingPutsEveryTheirWorkspaceBackInTheFeed(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	// The fixture Users are verified; Initech's owners start unverified here.
	for _, id := range []int64{fixtures.InitechOwnerOlgaID, fixtures.InitechCoOwnerOwenID} {
		require.NoError(t, h.env.DB.User.UpdateOneID(id).ClearEmailVerifiedAt().Exec(t.Context()))
	}
	_, saved := feedFrom(t, c, "")

	verifiedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, h.env.DB.User.UpdateOneID(fixtures.InitechOwnerOlgaID).SetEmailVerifiedAt(verifiedAt).Exec(t.Context()))
	got, saved := feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.InitechPublicID}, feedIDs(got))
	for _, o := range got[0].Owners {
		if o.UserId == 5 {
			at, ok := o.EmailVerifiedAt.Get()
			require.True(t, ok)
			assert.WithinDuration(t, verifiedAt, time.Time(at), time.Second)
		} else {
			assert.False(t, o.EmailVerifiedAt.IsSet() && !o.EmailVerifiedAt.IsNull(), "the co-owner stays unverified")
		}
	}

	require.NoError(t, h.env.DB.User.UpdateOneID(fixtures.OwnerJohnID).SetEmail("john@acme.test").Exec(t.Context()))
	got, saved = feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.AcmePublicID}, feedIDs(got))
	assert.Equal(t, "john@acme.test", string(got[0].Owners[0].Email))

	// A User who owns nothing is nobody's change.
	require.NoError(t, h.env.DB.User.UpdateOneID(fixtures.MemberMaryID).SetEmail("mary@elsewhere.test").Exec(t.Context()))
	got, _ = feedFrom(t, c, saved)
	assert.Empty(t, got)
}

func TestTheFeedNeedsTheReadWorkspacesScope(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.ServiceClient(t, readOnly).ServiceWorkspaceFeedList(t.Context(), serviceapi.ServiceWorkspaceFeedListParams{})

	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceWorkspaceFeedListForbidden{}, res, "a credential without workspaces:read is refused")
}

func TestAMalformedCursorIsRefused(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.ServiceClient(t, primary).ServiceWorkspaceFeedList(t.Context(),
		serviceapi.ServiceWorkspaceFeedListParams{Cursor: serviceapi.NewOptString("not-a-cursor")})

	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceWorkspaceFeedListBadRequest{}, res)
}

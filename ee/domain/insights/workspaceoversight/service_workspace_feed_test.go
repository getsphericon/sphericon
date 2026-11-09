package workspaceoversight_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entmembership "github.com/getsphericon/sphericon/ent/membership"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/userdeletion"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
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
	page, err := c.ServiceWorkspaceFeedList(t.Context(), params)
	require.NoError(t, err)
	return page
}

// feedFrom reads every page from cursor and returns the entries and the cursor to save.
func feedFrom(t *testing.T, c *serviceapi.Client, cursor string) (items []serviceapi.ServiceWorkspaceResource, next string) {
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
	for i := range entries {
		out[i] = string(entries[i].ID)
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
	for i := range entries {
		if string(entries[i].ID) == id {
			return entries[i]
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
	emails := map[int64]string{5: fixtures.InitechOwnerOlgaEmail, 6: fixtures.InitechCoOwnerOwenEmail}
	for _, o := range initech.Owners {
		assert.Equal(t, emails[int64(o.UserId)], string(o.Email))
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

	rest := feedPage(t, c, first.NextCursor, 50)
	assert.False(t, rest.HasMore)
	assert.NotEmpty(t, rest.Items)
	assert.Equal(t, rest, feedPage(t, c, first.NextCursor, 50), "and so is the page after a saved cursor")

	end := rest.NextCursor
	empty := feedPage(t, c, end, 50)
	assert.Empty(t, empty.Items, "nothing changed since the saved cursor")
	assert.Equal(t, end, empty.NextCursor, "an empty page keeps the cursor")
}

func TestANewWorkspaceEntersTheFeedWithItsOwner(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	_, saved := feedFrom(t, c, "")

	created, err := h.env.WorkspaceView.Create(t.Context(), workspaceview.CreateInput{UserID: fixtures.MemberMaryID, Name: "Hooli"})
	require.NoError(t, err)
	ws := created.Summary.Workspace

	got, _ := feedFrom(t, c, saved)
	require.Equal(t, []string{ws.PublicID}, feedIDs(got))
	assert.Equal(t, []int64{3}, ownerIDs(got[0]))
	assert.Equal(t, fixtures.MemberMaryEmail, string(got[0].Owners[0].Email))
}

func TestAnOwnerAddedRemovedOrTransferredPutsTheWorkspaceBackInTheFeed(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	initech := h.env.FeedDB.Scoped(fixtures.InitechID)
	_, saved := feedFrom(t, c, "")

	// Added.
	// test writes: the Workspace feed hook fires on this write
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

func TestADeletedOwnerLeavesTheFeedAndTheirWorkspaceComesBackWithoutThem(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	_, saved := feedFrom(t, c, "")

	_, err := h.env.UserDeletion.Delete(t.Context(), userdeletion.Request{UserID: fixtures.InitechCoOwnerOwenID, Email: fixtures.InitechCoOwnerOwenEmail, Password: fixtures.InitechCoOwnerOwenPassword})
	require.NoError(t, err)

	got, _ := feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.InitechPublicID}, feedIDs(got))
	assert.Equal(t, []int64{fixtures.InitechOwnerOlgaID}, ownerIDs(got[0]))
}

func TestAMemberJoiningOrLeavingIsNotAChangeOfOwners(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	initech := h.env.FeedDB.Scoped(fixtures.InitechID)
	_, saved := feedFrom(t, c, "")

	// test writes: the Workspace feed hook fires on this write
	m, err := initech.Membership().Create().SetUserID(fixtures.MemberMaryID).SetRole(entmembership.RoleMember).Save(t.Context())
	require.NoError(t, err)
	require.NoError(t, initech.Membership().DeleteOneID(m.ID).Exec(t.Context()))

	got, _ := feedFrom(t, c, saved)
	assert.Empty(t, got)
}

func TestAnOwnersEmailOrVerificationChangingPutsEveryTheirWorkspaceBackInTheFeed(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	// Neither of Virtucon's two owners has verified their email yet.
	_, saved := feedFrom(t, c, "")

	verifiedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, h.env.FeedDB.User.UpdateOneID(fixtures.VirtuconOwnerVeraID).SetEmailVerifiedAt(verifiedAt).Exec(t.Context()))
	got, saved := feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.VirtuconPublicID}, feedIDs(got))
	vera, foundVera := lo.Find(got[0].Owners, func(o serviceapi.ServiceWorkspaceOwner) bool { return o.UserId == fixtures.VirtuconOwnerVeraID })
	vic, foundVic := lo.Find(got[0].Owners, func(o serviceapi.ServiceWorkspaceOwner) bool { return o.UserId == fixtures.VirtuconCoOwnerVicID })
	require.True(t, foundVera)
	require.True(t, foundVic)
	at, verified := vera.EmailVerifiedAt.Get()
	require.True(t, verified)
	assert.WithinDuration(t, verifiedAt, time.Time(at), time.Second)
	assert.False(t, vic.EmailVerifiedAt.IsSet() && !vic.EmailVerifiedAt.IsNull(), "the co-owner stays unverified")

	require.NoError(t, h.env.FeedDB.User.UpdateOneID(fixtures.OwnerJohnID).SetEmail("john@acme.test").Exec(t.Context()))
	got, saved = feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.AcmePublicID}, feedIDs(got))
	assert.Equal(t, "john@acme.test", string(got[0].Owners[0].Email))

	// A User who owns nothing is nobody's change.
	require.NoError(t, h.env.FeedDB.User.UpdateOneID(fixtures.MemberMaryID).SetEmail("mary@elsewhere.test").Exec(t.Context()))
	got, _ = feedFrom(t, c, saved)
	assert.Empty(t, got)
}

func TestTheFeedNeedsTheReadWorkspacesScope(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.ServiceClient(t, readOnly).ServiceWorkspaceFeedList(t.Context(), serviceapi.ServiceWorkspaceFeedListParams{})

	testhelper.ServiceRefusal(t, err, http.StatusForbidden, "a credential without workspaces:read is refused")
}

func TestAMalformedCursorIsRefused(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.ServiceClient(t, primary).ServiceWorkspaceFeedList(t.Context(),
		serviceapi.ServiceWorkspaceFeedListParams{Cursor: serviceapi.NewOptString("not-a-cursor")})

	testhelper.ServiceRefusal(t, err, http.StatusBadRequest)
}

func TestDeletingAndRestoringAWorkspaceEachPutItBackInTheFeedInOrder(t *testing.T) {
	h := newHarness(t)
	c := h.env.ServiceClient(t, primary)
	now := time.Now().UTC().Truncate(time.Second)
	actor := events.Actor{Kind: events.ActorUser}
	initech := h.env.DB.Scoped(fixtures.InitechID)
	ws, err := initech.Workspace(t.Context())
	require.NoError(t, err)
	got, saved := feedFrom(t, c, "")
	live := entry(t, got, fixtures.InitechPublicID)
	assert.False(t, live.DiscardedAt.IsSet())
	assert.False(t, live.ErasedAt.IsSet())

	_, err = workspaces.New(h.env.DB, h.env.Bus, func() time.Time { return now }).Delete(t.Context(), initech, actor, ws.Name)
	require.NoError(t, err)
	got, afterDelete := feedFrom(t, c, saved)
	require.Equal(t, []string{fixtures.InitechPublicID}, feedIDs(got))
	discardedAt, ok := got[0].DiscardedAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, now, time.Time(discardedAt), time.Second)
	assert.False(t, got[0].ErasedAt.IsSet())
	assert.NotEmpty(t, got[0].Owners)

	// Replaying the cursor returns the same entries.
	again, replayed := feedFrom(t, c, saved)
	assert.Equal(t, got, again)
	assert.Equal(t, afterDelete, replayed)

	_, err = h.env.WorkspaceView.Restore(t.Context(), workspaceview.RestoreInput{UserID: 5, PublicID: fixtures.InitechPublicID, Actor: actor})
	require.NoError(t, err)
	got, _ = feedFrom(t, c, afterDelete)
	require.Equal(t, []string{fixtures.InitechPublicID}, feedIDs(got))
	assert.False(t, got[0].DiscardedAt.IsSet())
}

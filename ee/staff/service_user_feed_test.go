package staff_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/signup"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// controlPlane is a client of the operator's control plane: a Service credential that reads the
// Users feed.
func controlPlane(t *testing.T, h *harness) *serviceapi.Client {
	t.Helper()
	return h.env.ServiceClient(t, h.env.IssueServiceCredential(t, "control-plane", serviceapi.ServiceScopeUsersRead))
}

func userPage(t *testing.T, c *serviceapi.Client, cursor string, limit int32) *serviceapi.ServiceUserFeedPage {
	t.Helper()
	params := serviceapi.ServiceUserFeedListParams{}
	if cursor != "" {
		params.Cursor = serviceapi.NewOptString(cursor)
	}
	if limit > 0 {
		params.Limit = serviceapi.NewOptInt32(limit)
	}
	res, err := c.ServiceUserFeedList(t.Context(), params)
	require.NoError(t, err)
	page, ok := res.(*serviceapi.ServiceUserFeedPage)
	require.Truef(t, ok, "the feed is answered, got %T", res)
	return page
}

// usersFrom reads every page from cursor and returns the entries and the cursor to save.
func usersFrom(t *testing.T, c *serviceapi.Client, cursor string) (items []serviceapi.ServiceUserResource, next string) {
	t.Helper()
	var all []serviceapi.ServiceUserResource
	for {
		page := userPage(t, c, cursor, 5)
		all = append(all, page.Items...)
		cursor = page.NextCursor
		if !page.HasMore {
			return all, cursor
		}
	}
}

func userIDs(entries []serviceapi.ServiceUserResource) []int64 {
	return lo.Map(entries, func(e serviceapi.ServiceUserResource, _ int) int64 { return int64(e.ID) })
}

func userEntry(t *testing.T, entries []serviceapi.ServiceUserResource, id int64) serviceapi.ServiceUserResource {
	t.Helper()
	found, ok := lo.Find(entries, func(e serviceapi.ServiceUserResource) bool { return int64(e.ID) == id })
	require.Truef(t, ok, "no User %d in %v", id, userIDs(entries))
	return found
}

// markMailings makes the fixture OperatorNews item, which MemberMary declined at sign-up, the one
// that stands for consent to the operator's mailings, as a Staff member does in the console.
func markMailings(t *testing.T, h *harness) {
	t.Helper()
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.OperatorNewsID)
	_, err := signupitems.New(h.env.DB, h.env.Bus, nil).Update(t.Context(), fixtures.OperatorNewsID, signupitems.Update{MailingsConsent: new(true)})
	require.NoError(t, err)
}

// decide records the User's decision on the mailings item, as account settings do.
func decide(t *testing.T, h *harness, userID int64, decision consentrecord.Decision) {
	t.Helper()
	err := h.env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		return consentrecords.Write(t.Context(), tx, userID, consentrecords.Consents{
			Source:  consentrecord.SourceAccountSettings,
			Entries: []consentrecords.Entry{{VersionID: fixtures.OperatorNewsCurrentID, Decision: decision}},
		})
	})
	require.NoError(t, err)
}

func TestTheUserFeedFromAnEmptyCursorListsEveryUser(t *testing.T) {
	h := newHarness(t)

	entries, _ := usersFrom(t, controlPlane(t, h), "")

	want, err := h.env.DB.User.Query().IDs(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, want, userIDs(entries), "every User is in the feed once")
	john := userEntry(t, entries, fixtures.OwnerJohnID)
	assert.Equal(t, fixtures.OwnerJohnEmail, string(john.Email.Value))
	assert.NotEmpty(t, john.Name.Value)
	assert.False(t, john.DeletedAt.IsSet())
	assert.False(t, john.MailingsConsent, "no item stands for the mailings consent")
	assert.False(t, john.MailingsConsentChangedAt.IsSet())
}

func TestReplayingAUserFeedCursorReturnsTheSamePage(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	first := userPage(t, c, "", 2)
	require.Len(t, first.Items, 2)
	require.True(t, first.HasMore)

	again := userPage(t, c, "", 2)
	assert.Equal(t, first, again, "a page read twice from the same cursor is the same page")

	_, end := usersFrom(t, c, first.NextCursor)
	empty := userPage(t, c, end, 50)
	assert.Empty(t, empty.Items, "nothing changed since the saved cursor")
	assert.Equal(t, end, empty.NextCursor, "an empty page keeps the cursor")
}

func TestANewRenamedOrReaddressedUserIsANewEntry(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	acc := accounts.New(h.env.DB, h.env.Bus, testhelper.Passwords(), h.env.ConsentRecords)
	_, saved := usersFrom(t, c, "")

	created, err := acc.SignUpVerified(t.Context(), signup.NewUser{Name: "Nora New", Email: "nora@new.test", PasswordHash: "!"})
	require.NoError(t, err)
	got, saved := usersFrom(t, c, saved)
	require.Equal(t, []int64{created.ID}, userIDs(got))
	assert.Equal(t, "Nora New", got[0].Name.Value)
	assert.Equal(t, "nora@new.test", string(got[0].Email.Value))

	name := "Nora Renamed"
	_, err = acc.ChangeProfile(t.Context(), created, accounts.ProfileChange{Name: &name}, nil)
	require.NoError(t, err)
	got, saved = usersFrom(t, c, saved)
	require.Equal(t, []int64{created.ID}, userIDs(got))
	assert.Equal(t, name, got[0].Name.Value)

	require.NoError(t, acc.ChangeEmail(t.Context(), created.ID, "nora@moved.test"))
	got, _ = usersFrom(t, c, saved)
	require.Equal(t, []int64{created.ID}, userIDs(got))
	assert.Equal(t, "nora@moved.test", string(got[0].Email.Value))
}

func TestEndingTheSessionsIsNotAnEntry(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	acc := accounts.New(h.env.DB, h.env.Bus, testhelper.Passwords(), h.env.ConsentRecords)
	_, saved := usersFrom(t, c, "")

	require.NoError(t, acc.EndSessions(t.Context(), fixtures.OwnerJohnID))

	got, _ := usersFrom(t, c, saved)
	assert.Empty(t, got)
}

func TestAMailingsDecisionIsANewEntryWithTheConsentAndItsTime(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	markMailings(t, h)
	_, saved := usersFrom(t, c, "")

	decide(t, h, fixtures.OwnerJohnID, consentrecord.DecisionAccepted)
	got, saved := usersFrom(t, c, saved)
	require.Equal(t, []int64{fixtures.OwnerJohnID}, userIDs(got))
	assert.True(t, got[0].MailingsConsent)
	at, ok := got[0].MailingsConsentChangedAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now(), time.Time(at), time.Minute)

	decide(t, h, fixtures.OwnerJohnID, consentrecord.DecisionWithdrawn)
	got, _ = usersFrom(t, c, saved)
	require.Equal(t, []int64{fixtures.OwnerJohnID}, userIDs(got))
	assert.False(t, got[0].MailingsConsent, "a withdrawal ends the consent")
	assert.True(t, got[0].MailingsConsentChangedAt.IsSet())
}

func TestASignUpThatAcceptsTheMailingsIsOneEntryWithTheConsent(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	markMailings(t, h)
	_, saved := usersFrom(t, c, "")
	consents, err := h.env.ConsentRecords.Check(t.Context(),
		[]consentrecords.Answer{{Key: fixtures.OperatorNewsKey, Version: fixtures.OperatorNewsCurrentLabel, Accepted: true}},
		consentrecord.SourceSignUpForm)
	require.NoError(t, err)

	created, err := accounts.New(h.env.DB, h.env.Bus, testhelper.Passwords(), h.env.ConsentRecords).
		SignUpVerified(t.Context(), signup.NewUser{Name: "Nora New", Email: "nora@new.test", PasswordHash: "!", Consents: consents})
	require.NoError(t, err)

	got, _ := usersFrom(t, c, saved)
	require.Equal(t, []int64{created.ID}, userIDs(got))
	assert.True(t, got[0].MailingsConsent)
	assert.True(t, got[0].MailingsConsentChangedAt.IsSet())
}

func TestADecisionOnAnotherItemIsNotAnEntry(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	markMailings(t, h)
	_, saved := usersFrom(t, c, "")

	err := h.env.Bus.WithinTx(t.Context(), func(tx *ent.Client, _ events.Publisher) error {
		return consentrecords.Write(t.Context(), tx, fixtures.OwnerJohnID, consentrecords.Consents{
			Source:  consentrecord.SourceReAcceptance,
			Entries: []consentrecords.Entry{{VersionID: fixtures.ServiceTermsCurrentID, Decision: consentrecord.DecisionAccepted}},
		})
	})
	require.NoError(t, err)

	got, _ := usersFrom(t, c, saved)
	assert.Empty(t, got)
}

func TestMarkingOrUnmarkingTheMailingsItemPutsItsAnswerersBackInTheFeed(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.OperatorNewsID)
	items := signupitems.New(h.env.DB, h.env.Bus, nil)
	_, saved := usersFrom(t, c, "")

	// MemberMary declined OperatorNews at sign-up: once it stands for the mailings consent, that
	// decline is her consent.
	_, err := items.Update(t.Context(), fixtures.OperatorNewsID, signupitems.Update{MailingsConsent: new(true)})
	require.NoError(t, err)
	got, saved := usersFrom(t, c, saved)
	require.Equal(t, []int64{fixtures.MemberMaryID}, userIDs(got))
	assert.False(t, got[0].MailingsConsent)
	assert.True(t, got[0].MailingsConsentChangedAt.IsSet(), "she has answered the item")

	_, err = items.Update(t.Context(), fixtures.OperatorNewsID, signupitems.Update{MailingsConsent: new(false)})
	require.NoError(t, err)
	got, _ = usersFrom(t, c, saved)
	require.Equal(t, []int64{fixtures.MemberMaryID}, userIDs(got))
	assert.False(t, got[0].MailingsConsentChangedAt.IsSet(), "no item stands for the mailings consent any more")
}

func TestADeletedUserIsATombstone(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	_, saved := usersFrom(t, c, "")

	_, err := accounts.New(h.env.DB, h.env.Bus, testhelper.Passwords(), h.env.ConsentRecords).DeleteUser(t.Context(), fixtures.MemberMaryID, nil)
	require.NoError(t, err)

	got, saved := usersFrom(t, c, saved)
	require.Equal(t, []int64{fixtures.MemberMaryID}, userIDs(got))
	deletedAt, ok := got[0].DeletedAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now(), time.Time(deletedAt), time.Minute)
	assert.False(t, got[0].Name.IsSet(), "nothing of the User is kept")
	assert.False(t, got[0].Email.IsSet())
	assert.False(t, got[0].MailingsConsent)

	again, _ := usersFrom(t, c, "")
	assert.Contains(t, userIDs(again), int64(fixtures.MemberMaryID), "the tombstone stays in the feed from the beginning")
	later, _ := usersFrom(t, c, saved)
	assert.Empty(t, later)
}

func TestTheFeedPagesFromATombstoneOnToTheUsersChangedAfterIt(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	acc := accounts.New(h.env.DB, h.env.Bus, testhelper.Passwords(), h.env.ConsentRecords)
	_, saved := usersFrom(t, c, "")
	_, err := acc.DeleteUser(t.Context(), fixtures.MemberMaryID, nil)
	require.NoError(t, err)
	john, err := h.env.DB.User.Get(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	name := "John Renamed"
	_, err = acc.ChangeProfile(t.Context(), john, accounts.ProfileChange{Name: &name}, nil)
	require.NoError(t, err)

	first := userPage(t, c, saved, 1)
	require.Equal(t, []int64{fixtures.MemberMaryID}, userIDs(first.Items))
	assert.True(t, first.Items[0].DeletedAt.IsSet())
	assert.True(t, first.HasMore)

	second := userPage(t, c, first.NextCursor, 1)
	require.Equal(t, []int64{fixtures.OwnerJohnID}, userIDs(second.Items))
	assert.Equal(t, name, second.Items[0].Name.Value)
	assert.False(t, second.HasMore)
}

func TestAPrunedUnverifiedUserIsATombstone(t *testing.T) {
	h := newHarness(t)
	c := controlPlane(t, h)
	_, saved := usersFrom(t, c, "")

	_, err := accounts.New(h.env.DB, h.env.Bus, testhelper.Passwords(), h.env.ConsentRecords).PruneUnverified(t.Context(), 7*24*time.Hour, 10)
	require.NoError(t, err)

	got, _ := usersFrom(t, c, saved)
	assert.Contains(t, userIDs(got), int64(fixtures.StaleUnverifiedUserID))
	assert.True(t, userEntry(t, got, fixtures.StaleUnverifiedUserID).DeletedAt.IsSet())
}

func TestTheUserFeedNeedsTheReadUsersScope(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.ServiceClient(t, primary).ServiceUserFeedList(t.Context(), serviceapi.ServiceUserFeedListParams{})

	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceUserFeedListForbidden{}, res, "a credential for Workspaces does not read Users")
}

func TestAMalformedUserFeedCursorIsRefused(t *testing.T) {
	h := newHarness(t)

	res, err := controlPlane(t, h).ServiceUserFeedList(t.Context(),
		serviceapi.ServiceUserFeedListParams{Cursor: serviceapi.NewOptString("not-a-cursor")})

	require.NoError(t, err)
	assert.IsType(t, &serviceapi.ServiceUserFeedListBadRequest{}, res)
}

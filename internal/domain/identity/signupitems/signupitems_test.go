package signupitems_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/instance/instanceauditentries"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setup(t *testing.T) (*testhelper.TestEnv, *signupitems.Module) {
	t.Helper()
	env := testhelper.Setup(t)
	return env, testhelper.SignupItems(env.DB, env.Bus)
}

// A key another item has, a deleted one's too, is refused by signup_items_key_key, and nothing is
// added.
func TestAddRefusesATakenKey(t *testing.T) {
	env, m := setup(t)
	before, err := env.DB.SignupItem.Query().Count(t.Context())
	require.NoError(t, err)

	_, err = m.Add(t.Context(), fixtures.StaffEnrolledID, signupitems.Input{Key: fixtures.ServiceTermsKey, Title: "Other terms", Kind: signupitem.KindNotice})

	require.ErrorIs(t, err, signupitems.ErrKeyTaken)
	after, err := env.DB.SignupItem.Query().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// One item stands for consent to the operator's mailings: signupitem_mailings_consent refuses a
// second, added or marked, and the first keeps the mark.
func TestASecondMailingsConsentIsRefused(t *testing.T) {
	env, m := setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.OperatorNewsID, fixtures.PartnerOffersID)
	_, err := m.Update(t.Context(), fixtures.StaffEnrolledID, fixtures.OperatorNewsID, signupitems.UpdateInput{MailingsConsent: new(true)})
	require.NoError(t, err)

	_, added := m.Add(t.Context(), fixtures.StaffEnrolledID, signupitems.Input{Key: "events", Title: "Events", Kind: signupitem.KindOptionalConsent, MailingsConsent: true})
	_, marked := m.Update(t.Context(), fixtures.StaffEnrolledID, fixtures.PartnerOffersID, signupitems.UpdateInput{MailingsConsent: new(true)})

	require.ErrorIs(t, added, signupitems.ErrMailingsTaken)
	require.ErrorIs(t, marked, signupitems.ErrMailingsTaken)
	news, err := m.Get(t.Context(), fixtures.OperatorNewsID)
	require.NoError(t, err)
	assert.True(t, news.MailingsConsent, "the first item keeps the mark")
}

// A label the item has already published under is refused by legaldocumentversion_item_id_label.
func TestPublishRefusesALabelTheItemHas(t *testing.T) {
	env, m := setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	_, err := m.SaveDraft(t.Context(), fixtures.ServiceTermsID, "a new text")
	require.NoError(t, err)

	_, err = m.Publish(t.Context(), fixtures.ServiceTermsID, fixtures.ServiceTermsCurrentLabel, fixtures.StaffEnrolledID)

	require.ErrorIs(t, err, signupitems.ErrLabelTaken)
	doc, err := m.Document(t.Context(), fixtures.ServiceTermsKey)
	require.NoError(t, err)
	assert.NotEqual(t, "a new text", doc.Markdown, "the published version is unchanged")
}

// The writes that read the items' positions queue behind the items' lock: two Reorders in opposite
// orders both finish instead of deadlocking, and two Adds take positions of their own.
func TestPositionWritesQueueBehindTheItemsLock(t *testing.T) {
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	m := testhelper.SignupItems(testhelper.Clients(t).New(race.DB), race.Bus)
	keys := []string{"privacy", "offers"}
	ids := make([]int64, 0, len(keys))
	for _, key := range keys {
		added, err := m.Add(t.Context(), fixtures.StaffEnrolledID, signupitems.Input{Key: key, Title: key, Kind: signupitem.KindNotice})
		require.NoError(t, err)
		ids = append(ids, added.ID)
	}
	reversed := slices.Clone(ids)
	slices.Reverse(reversed)

	errs := race.QueueBehindKey(t, signupitems.LockKey,
		testhelper.Writer{Name: "in order", Run: func() error { _, err := m.Reorder(t.Context(), fixtures.StaffEnrolledID, ids); return err }},
		testhelper.Writer{Name: "reversed", Run: func() error { _, err := m.Reorder(t.Context(), fixtures.StaffEnrolledID, reversed); return err }},
		testhelper.Writer{Name: "first add", Run: func() error {
			_, err := m.Add(t.Context(), fixtures.StaffEnrolledID, signupitems.Input{Key: "first", Title: "First", Kind: signupitem.KindNotice})
			return err
		}},
		testhelper.Writer{Name: "second add", Run: func() error {
			_, err := m.Add(t.Context(), fixtures.StaffEnrolledID, signupitems.Input{Key: "second", Title: "Second", Kind: signupitem.KindNotice})
			return err
		}},
	)

	for name, err := range errs {
		require.NoError(t, err, name)
	}
	after, err := m.List(t.Context())
	require.NoError(t, err)
	positions := lo.Map(after, func(i signupitems.Item, _ int) int { return i.Position })
	assert.Len(t, lo.Uniq(positions), len(positions), "no two items share a position")
}

// Each change of the Sign-up items is recorded in the Instance audit log as its Staff member's,
// naming the item by its key (ADR 0070); a change that changes nothing records nothing.
func TestEveryChangeOfTheItemsIsRecordedInTheInstanceAuditLog(t *testing.T) {
	env, m := setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.OperatorNewsID)
	ctx := t.Context()
	staff := int64(fixtures.StaffEnrolledID)

	added, err := m.Add(ctx, staff, signupitems.Input{Key: "events", Title: "Events", Kind: signupitem.KindNotice})
	require.NoError(t, err)
	_, err = m.Update(ctx, staff, added.ID, signupitems.UpdateInput{Title: new("Events")})
	require.NoError(t, err, "the same title changes nothing")
	_, err = m.Update(ctx, staff, added.ID, signupitems.UpdateInput{Title: new("Upcoming events")})
	require.NoError(t, err)
	_, err = m.Reorder(ctx, staff, []int64{added.ID, fixtures.OperatorNewsID})
	require.NoError(t, err)
	_, err = m.Reorder(ctx, staff, []int64{added.ID, fixtures.OperatorNewsID})
	require.NoError(t, err, "the same order changes nothing")
	_, err = m.SaveDraft(ctx, added.ID, "the text")
	require.NoError(t, err)
	_, err = m.Publish(ctx, added.ID, "v1", staff)
	require.NoError(t, err)
	require.NoError(t, m.Delete(ctx, staff, added.ID))

	page, err := instanceauditentries.Entries(ctx, env.DB, "", 20)
	require.NoError(t, err)
	ours := lo.Filter(page.Entries, func(e *ent.InstanceAuditEntry, _ int) bool {
		return e.Action != instanceauditentries.ActionDefaultSendCapChange // the fixtures' own entries
	})
	slices.Reverse(ours)
	got := lo.Map(ours, func(e *ent.InstanceAuditEntry, _ int) string {
		assert.Equal(t, staff, e.StaffMemberID)
		diff, err := json.Marshal(e.Diff)
		require.NoError(t, err)
		return e.Action + " " + string(diff)
	})
	news := fixtures.OperatorNewsKey
	assert.Equal(t, []string{
		`instance.signup_item_add {"key":{"to":"events"},"kind":{"to":"notice"},"mailings_consent":{"to":false},"title":{"to":"Events"}}`,
		`instance.signup_item_change {"key":{"from":"events","to":"events"},"title":{"from":"Events","to":"Upcoming events"}}`,
		`instance.signup_items_reorder {"order":{"from":["` + news + `","events"],"to":["events","` + news + `"]}}`,
		`instance.legal_document_publish {"key":{"from":"events","to":"events"},"version":{"to":"v1"}}`,
		`instance.signup_item_delete {"key":{"from":"events"}}`,
	}, got)
}

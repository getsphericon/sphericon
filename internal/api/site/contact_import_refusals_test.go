package site_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/membership"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

// refusalColumn is a refusal column of channel whose values each record refusal.
func refusalColumn(channel siteapi.ContactImportRefusalChannel, refusal siteapi.ContactImportRefusal, values ...string) siteapi.ContactImportColumnInput {
	return siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportRefusalColumnInputContactImportColumnInputSum(siteapi.ContactImportRefusalColumnInput{
		Kind: siteapi.ContactImportRefusalColumnInputKindRefusal, Channel: channel,
		Values: lo.Map(values, func(v string, _ int) siteapi.ContactImportRefusalValueInput {
			return siteapi.ContactImportRefusalValueInput{Value: v, Refusal: refusal}
		}),
	})}
}

// timeColumn is a time column of the column of, written in RFC 3339.
func timeColumn(of int32) siteapi.ContactImportColumnInput {
	return siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportTimeColumnInputContactImportColumnInputSum(siteapi.ContactImportTimeColumnInput{
		Kind: siteapi.ContactImportTimeColumnInputKindTime, Of: siteapi.NewNilInt32(of), DateFormat: siteapi.NewNilContactImportDateFormat(siteapi.ContactImportDateFormatRfc3339),
	})}
}

// emailStates are the email standings of Acme's Contacts of addresses, by address.
func emailStates(t *testing.T, env *testhelper.TestEnv, addresses ...string) map[string]eligibility.ChannelState {
	t.Helper()
	contacts := lo.Map(addresses, func(a string, _ int) *ent.Contact {
		c := contactByEmail(t, env, a)
		require.NotNil(t, c, a)
		return c
	})
	states, err := eligibility.New().ChannelStates(t.Context(), env.DB.Scoped(fixtures.AcmeID), contacts)
	require.NoError(t, err)
	return lo.SliceToMap(contacts, func(c *ent.Contact) (string, eligibility.ChannelState) {
		return lo.FromPtr(c.Email), states[c.ID].Email
	})
}

// Only the values a refusal column lists record an opt-out, whatever their case and spaces, of the
// scope the mapping chooses and `via: imported`; any other value records nothing.
func TestAnImportedUnsubscribeOptsOutTheListedValuesOnly(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,status\nann.moved@example.com,unsubscribed\nbob.moved@example.com,subscribed\ncarl.moved@example.com, Unsubscribed \ndora.moved@example.com,\n")
	in := mappingInput(t, imp)
	in.Columns[1] = refusalColumn(siteapi.ContactImportRefusalChannelEmail, siteapi.ContactImportRefusalUnsubscribeEverything, "unsubscribed")

	done := runImport(t, env, owner, imp, in)
	states := emailStates(t, env, "ann.moved@example.com", "bob.moved@example.com", "carl.moved@example.com", "dora.moved@example.com")
	unsubscribed := env.DB.Event.Query().Where(
		event.WorkspaceID(fixtures.AcmeID), event.Action(systemevent.EmailUnsubscribed), event.EmailHasSuffix(".moved@example.com"),
	).AllX(t.Context())

	assert.Equal(t, siteapi.ContactImportCounts{Processed: 4, Created: 4, OptedOut: 2}, done.Counts)
	assert.Equal(t, eligibility.StateOptedOut, states["ann.moved@example.com"].State)
	assert.Equal(t, []string{eligibility.SourceEverything}, lo.Map(states["ann.moved@example.com"].OptOuts, func(o eligibility.OptOut, _ int) string { return o.Scope }))
	assert.Equal(t, eligibility.StateOptedOut, states["carl.moved@example.com"].State)
	assert.Equal(t, eligibility.StateMayReceive, states["bob.moved@example.com"].State)
	assert.Equal(t, eligibility.StateMayReceive, states["dora.moved@example.com"].State)
	assert.ElementsMatch(t, []string{"ann.moved@example.com", "carl.moved@example.com"}, lo.Map(unsubscribed, func(e *ent.Event, _ int) string { return lo.FromPtr(e.Email) }))
	for _, e := range unsubscribed {
		assert.Equal(t, events.UnsubscribedByImported, e.Properties["via"])
		assert.True(t, events.WasHistory(e), "the import's history mark holds it back as a trigger")
	}
}

// A bounced value records a Suppression of the manual reason, counted apart from the opt-outs;
// an opt-out of broadcasts leaves the address reachable by its Automations.
func TestAnImportedBounceSuppressesAndABroadcastsOptOutIsNarrow(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,bounced,newsletter\nann.moved@example.com,yes,\nbob.moved@example.com,,unsubscribed\n")
	in := mappingInput(t, imp)
	in.Columns[1] = refusalColumn(siteapi.ContactImportRefusalChannelEmail, siteapi.ContactImportRefusalSuppression, "yes")
	in.Columns[2] = refusalColumn(siteapi.ContactImportRefusalChannelEmail, siteapi.ContactImportRefusalUnsubscribeBroadcasts, "unsubscribed")

	done := runImport(t, env, owner, imp, in)
	states := emailStates(t, env, "ann.moved@example.com", "bob.moved@example.com")

	assert.Equal(t, siteapi.ContactImportCounts{Processed: 2, Created: 2, OptedOut: 1, Suppressed: 1}, done.Counts)
	assert.Equal(t, eligibility.StateSuppressed, states["ann.moved@example.com"].State)
	require.NotNil(t, states["ann.moved@example.com"].Suppression)
	assert.Equal(t, entsuppression.ReasonManual, states["ann.moved@example.com"].Suppression.Reason)
	assert.Equal(t, []string{eligibility.SourceBroadcasts}, lo.Map(states["bob.moved@example.com"].OptOuts, func(o eligibility.OptOut, _ int) string { return o.Scope }))
}

// A Telegram refusal is recorded at the Bindings of the Contact its row writes, of the scope it
// had: an opt-out of broadcasts stays one of broadcasts, as a Telegram link records it. A row
// whose Contact has no Binding records nothing and is reported; the preview, which looks up no
// Binding, counts it as one that may.
func TestAnImportedTelegramRefusalIsRecordedAtTheContactsBindings(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,telegram\n"+fixtures.ContactLiamEmail+",broadcasts\nann.moved@example.com,broadcasts\n")
	in := mappingInput(t, imp)
	in.Columns[1] = refusalColumn(siteapi.ContactImportRefusalChannelTelegram, siteapi.ContactImportRefusalUnsubscribeBroadcasts, "broadcasts")
	_, err := saveMapping(t, owner, imp, in)
	require.NoError(t, err)
	preview, err := owner.SiteContactImportsPreview(ctx, siteapi.SiteContactImportsPreviewParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
	require.NoError(t, err)

	done := runImport(t, env, owner, imp, in)
	liam := contactByEmail(t, env, fixtures.ContactLiamEmail)
	require.NotNil(t, liam)
	states, err := eligibility.New().ChannelStates(ctx, env.DB.Scoped(fixtures.AcmeID), []*ent.Contact{liam})
	require.NoError(t, err)

	assert.Equal(t, int32(2), preview.OptOuts, "up to one a row")
	assert.Equal(t, int32(1), done.Counts.OptedOut)
	assert.Equal(t, int32(1), done.ReportRows, "the row whose Contact has no Binding")
	assert.Equal(t, []string{eligibility.SourceBroadcasts}, lo.Map(states[liam.ID].Telegram.OptOuts, func(o eligibility.OptOut, _ int) string { return o.Scope }))
}

// A mapping that records a Suppression needs suppressions:manage, at saving and at the run; one
// that records only opt-outs does not. An opt-out of broadcasts on SMS is refused.
func TestARefusalColumnRecordingASuppressionNeedsSuppressionsManage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,status\nann.moved@example.com,bounced\n")
	imports := testhelper.Module[*contactimports.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	mapping := func(refusal csvimport.Refusal, channel channels.Channel) csvimport.Mapping {
		return csvimport.Mapping{DecimalSeparator: csvimport.DecimalPoint, Columns: []csvimport.Column{
			{Kind: csvimport.KindKey, Key: "email"},
			{Kind: csvimport.KindRefusal, Fact: &csvimport.Fact{Channel: channel, Refusals: []csvimport.RefusalValue{{Value: "bounced", Refusal: refusal}}}},
		}}
	}

	_, suppressing := imports.SaveMapping(ctx, acme, int64(imp.ID), contactimports.MappingInput{Mapping: mapping(csvimport.RefusalSuppression, channels.Email), Role: membership.RoleMember})
	_, smsBroadcasts := imports.SaveMapping(ctx, acme, int64(imp.ID), contactimports.MappingInput{Mapping: mapping(csvimport.RefusalBroadcasts, channels.SMS), Role: membership.RoleOwner})
	_, optingOut := imports.SaveMapping(ctx, acme, int64(imp.ID), contactimports.MappingInput{Mapping: mapping(csvimport.RefusalEverything, channels.Email), Role: membership.RoleMember})
	_, err := imports.SaveMapping(ctx, acme, int64(imp.ID), contactimports.MappingInput{Mapping: mapping(csvimport.RefusalSuppression, channels.Email), Role: membership.RoleOwner})
	require.NoError(t, err)
	_, runByMember := imports.Run(ctx, acme, int64(imp.ID), membership.RoleMember)

	lacking := memberships.Require(membership.RoleMember, memberships.PermSuppressionsManage)
	require.ErrorIs(t, suppressing, lacking)
	require.ErrorIs(t, smsBroadcasts, contactimports.ErrBroadcastsNotOnSMS)
	require.NoError(t, optingOut)
	require.ErrorIs(t, runByMember, lacking)
}

// A refusal is recorded on a Contact the Workspace has even when only new Contacts are added.
func TestAnImportedRefusalIsRecordedWhenOnlyNewContactsAreAdded(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,first_name,status\n"+fixtures.ContactAliceEmail+",Alicia,unsubscribed\n")
	in := mappingInput(t, imp)
	in.OnlyAddNew = true
	in.Columns[2] = refusalColumn(siteapi.ContactImportRefusalChannelEmail, siteapi.ContactImportRefusalUnsubscribeEverything, "unsubscribed")

	done := runImport(t, env, owner, imp, in)
	alice := contactByEmail(t, env, fixtures.ContactAliceEmail)
	states := emailStates(t, env, fixtures.ContactAliceEmail)

	assert.Equal(t, siteapi.ContactImportCounts{Processed: 1, Updated: 1, OptedOut: 1}, done.Counts)
	assert.Equal(t, "Alice", lo.FromPtr(alice.FirstName))
	assert.Equal(t, eligibility.StateOptedOut, states[fixtures.ContactAliceEmail].State)
}

// An unsubscribe dated before the person's later return changes nothing (ADR 0154); one dated in
// the future is recorded as of now and reported, as is one with no time.
func TestAnImportedUnsubscribeIsOrderedByItsTime(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	left := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	_, err := unsubscriberecording.New(env.Bus, env.OutboundModule(), testhelper.Module[*automations.Module](env), testhelper.Module[*unsubscribes.Module](env),
		testhelper.Module[*confirmations.Module](env), testhelper.Module[*channelstop.Module](env), func() time.Time { return left }).
		Unsubscribe(ctx, acme, fixtures.ContactAliceEmail, eligibility.SourceEverything)
	require.NoError(t, err)
	require.NoError(t, confirmations.New(env.Bus, unsubscribes.New(), func() time.Time { return left.Add(24 * time.Hour) }).
		RecordConfirmation(ctx, acme, linktarget.Confirmation{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Destination: fixtures.ContactAliceEmail}, ""))
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,status,left at\n"+
		fixtures.ContactAliceEmail+",unsubscribed,"+left.Add(time.Hour).Format(time.RFC3339)+"\n"+
		"ann.moved@example.com,unsubscribed,2999-01-01T00:00:00Z\n"+
		"bob.moved@example.com,unsubscribed,\n")
	in := mappingInput(t, imp)
	in.Columns[1] = refusalColumn(siteapi.ContactImportRefusalChannelEmail, siteapi.ContactImportRefusalUnsubscribeEverything, "unsubscribed")
	in.Columns[2] = timeColumn(1)
	before := time.Now()

	done := runImport(t, env, owner, imp, in)
	states := emailStates(t, env, fixtures.ContactAliceEmail, "ann.moved@example.com", "bob.moved@example.com")

	assert.NotEqual(t, eligibility.StateOptedOut, states[fixtures.ContactAliceEmail].State, "the return came after")
	assert.Empty(t, states[fixtures.ContactAliceEmail].OptOuts)
	require.Len(t, states["ann.moved@example.com"].OptOuts, 1)
	assert.False(t, states["ann.moved@example.com"].OptOuts[0].At.Before(before.Add(-time.Second)), "stamped now")
	assert.Equal(t, eligibility.StateOptedOut, states["bob.moved@example.com"].State)
	assert.Equal(t, int32(2), done.Counts.Dropped)
	assert.Equal(t, int32(2), done.ReportRows, "the two rows whose time was dropped")
}

// A consent column records an imported email Confirmation with its evidence for a cell that is one
// of its values as a whole; over a standing opt-out it lifts nothing.
func TestAnImportedConsentIsAnImportedConfirmationThatLiftsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, acme, fixtures.ContactAliceEmail, eligibility.SourceEverything)
	require.NoError(t, err)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,opted in,form\nann.moved@example.com,Yes,signup page\n"+fixtures.ContactAliceEmail+",yes,signup page\nbob.moved@example.com,no,signup page\ndora.moved@example.com,\"yes, no\",signup page\n")
	in := mappingInput(t, imp)
	in.Columns[1] = siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportConsentColumnInputContactImportColumnInputSum(siteapi.ContactImportConsentColumnInput{
		Kind: siteapi.ContactImportConsentColumnInputKindConsent, Channel: siteapi.ContactImportConsentChannelEmail, Values: []string{"yes"},
		Evidence: siteapi.ContactImportConsentEvidenceInput{
			Wording: siteapi.NewNilString("Send me the newsletter"), WordingColumn: siteapi.NilInt32{Null: true},
			Source: siteapi.NilString{Null: true}, SourceColumn: siteapi.NewNilInt32(2),
			Brand: siteapi.NewNilString("Acme"), BrandColumn: siteapi.NilInt32{Null: true},
		},
	})}

	runImport(t, env, owner, imp, in)
	states := emailStates(t, env, "ann.moved@example.com", fixtures.ContactAliceEmail, "bob.moved@example.com", "dora.moved@example.com")

	require.NotNil(t, states["ann.moved@example.com"].Confirmation)
	assert.Equal(t, confirmation.ProvenanceImported, states["ann.moved@example.com"].Confirmation.Provenance)
	assert.Nil(t, states["bob.moved@example.com"].Confirmation)
	assert.Nil(t, states["dora.moved@example.com"].Confirmation, "a consent value matches the whole cell, not an item of a list")
	assert.Equal(t, eligibility.StateOptedOut, states[fixtures.ContactAliceEmail].State)
}

// An Engagement tracking column records a grant or a refusal by the values it lists.
func TestAnImportedTrackingColumnRecordsGrantsAndRefusals(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,tracking\nann.moved@example.com,no\nbob.moved@example.com,yes\ncarl.moved@example.com,maybe\n")
	in := mappingInput(t, imp)
	in.Columns[1] = siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportEngagementTrackingColumnInputContactImportColumnInputSum(siteapi.ContactImportEngagementTrackingColumnInput{
		Kind: siteapi.ContactImportEngagementTrackingColumnInputKindEngagementTracking, Channel: siteapi.ContactImportConsentChannelEmail,
		Granted: []string{"yes"}, Denied: []string{"no"},
	})}

	runImport(t, env, owner, imp, in)
	tracking := testhelper.Module[*engagementtracking.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	read := func(address string) engagementtracking.Reading {
		c, err := tracking.Of(t.Context(), acme, channels.Email, address)
		require.NoError(t, err)
		return c.State
	}

	assert.Equal(t, engagementtracking.ReadingDenied, read("ann.moved@example.com"))
	assert.Equal(t, engagementtracking.ReadingGranted, read("bob.moved@example.com"))
	assert.NotEqual(t, engagementtracking.ReadingGranted, read("carl.moved@example.com"))
	assert.NotEqual(t, engagementtracking.ReadingDenied, read("carl.moved@example.com"))
}

// The preview counts the opt-outs and Suppressions the whole file would record, at Destinations
// whose value matches, writing none of them.
func TestAContactImportPreviewCountsTheRefusalsOfTheWholeFile(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,status\nann.moved@example.com,unsubscribed\nbob.moved@example.com,bounced\n,unsubscribed\ncarl.moved@example.com,active\n")
	in := mappingInput(t, imp)
	in.Columns[1] = siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportRefusalColumnInputContactImportColumnInputSum(siteapi.ContactImportRefusalColumnInput{
		Kind: siteapi.ContactImportRefusalColumnInputKindRefusal, Channel: siteapi.ContactImportRefusalChannelEmail,
		Values: []siteapi.ContactImportRefusalValueInput{
			{Value: "unsubscribed", Refusal: siteapi.ContactImportRefusalUnsubscribeEverything},
			{Value: "bounced", Refusal: siteapi.ContactImportRefusalSuppression},
		},
	})}
	_, err := saveMapping(t, owner, imp, in)
	require.NoError(t, err)

	preview, err := owner.SiteContactImportsPreview(t.Context(), siteapi.SiteContactImportsPreviewParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
	require.NoError(t, err)

	assert.Equal(t, int32(1), preview.OptOuts)
	assert.Equal(t, int32(1), preview.Suppressions)
	assert.Nil(t, contactByEmail(t, env, "ann.moved@example.com"))
}

// An SMS refusal stops the row's number under the import's history mark as any STOP does.
func TestAnImportedSMSRefusalStopsTheNumber(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "phone,sms status\n"+fixtures.ContactTexterPhone+",STOPPED\n")
	in := mappingInput(t, imp)
	in.Columns[1] = refusalColumn(siteapi.ContactImportRefusalChannelSMS, siteapi.ContactImportRefusalUnsubscribeEverything, "stopped")

	done := runImport(t, env, owner, imp, in)
	texter, err := env.DB.Scoped(fixtures.AcmeID).Contact().Get(t.Context(), fixtures.ContactTexterID)
	require.NoError(t, err)
	states, err := eligibility.New().ChannelStates(t.Context(), env.DB.Scoped(fixtures.AcmeID), []*ent.Contact{texter})
	require.NoError(t, err)

	assert.Equal(t, int32(1), done.Counts.OptedOut)
	assert.Equal(t, eligibility.StateOptedOut, states[fixtures.ContactTexterID].SMS.State)
}

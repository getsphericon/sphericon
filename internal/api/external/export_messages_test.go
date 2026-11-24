package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func readExportMessages(t *testing.T, c *externalapi.Client, cursor string) exportPage {
	t.Helper()
	return readExportStream(t, c, messagesStream, cursor, 0)
}

func TestExportMessagesReadsDeliveryMetadataWithoutContent(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)

	page := readExportMessages(t, c, "")

	require.NotEmpty(t, page.cursor)
	broadcast := page.row(fixtures.OutboundMessageErasableBroadcastID)
	require.NotNil(t, broadcast)
	assert.Equal(t, "broadcast", broadcast["kind"])
	assert.Equal(t, "email", broadcast["channel"])
	assert.Equal(t, fixtures.OutboundMessageErasableBroadcastDestination, broadcast["destination"])
	assert.Equal(t, "sent", broadcast["status"])
	assert.Equal(t, "mail.initech.test", broadcast["sending_domain"])
	assert.NotNil(t, broadcast["contact_id"])
	assert.NotNil(t, broadcast["broadcast_id"])
	assert.NotNil(t, broadcast["sent_at"])
	assert.NotContains(t, broadcast, "subject")
	assert.NotContains(t, broadcast, "body")
	stranger := page.row(fixtures.OutboundMessageStrangerID)
	require.NotNil(t, stranger)
	assert.Nil(t, stranger["contact_id"])
}

func TestExportMessagesDeliversAStatusChangeAgain(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.GlobexID)
	before := readExportMessages(t, c, "")
	require.Equal(t, "pending", before.row(fixtures.OutboundMessageGlobexID)["status"])

	require.NoError(t, env.DB.Scoped(fixtures.GlobexID).OutboundMessage().
		UpdateOneID(fixtures.OutboundMessageGlobexID).
		SetStatus(outboundmessage.StatusSent).
		Exec(context.Background()))

	after := readExportMessages(t, c, before.cursor)
	changed := after.row(fixtures.OutboundMessageGlobexID)
	require.NotNil(t, changed, "the changed row comes again from the cursor taken before")
	assert.Equal(t, "sent", changed["status"])
}

func TestExportMessagesDeliversAnErasedDestinationAnonymized(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)
	before := readExportMessages(t, c, "")
	require.Equal(t, fixtures.OutboundMessageErasableBroadcastDestination,
		before.row(fixtures.OutboundMessageErasableBroadcastID)["destination"])

	eraseErasable(t, env)

	after := readExportMessages(t, c, before.cursor)
	for _, id := range []int64{fixtures.OutboundMessageErasableBroadcastID, fixtures.OutboundMessageErasableTransactionalID} {
		row := after.row(id)
		require.NotNilf(t, row, "message %d comes again", id)
		assert.Nil(t, row["destination"])
		assert.Nil(t, row["contact_id"])
	}
	assert.Nil(t, after.row(fixtures.OutboundMessageBystanderID), "an unrelated message is not redelivered")
}

func TestExportMessagesNeedsTheExportReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	for _, scopes := range [][]apitokens.Scope{{}, {apitokens.ScopeContactsRead}, {apitokens.ScopeContactsRead, apitokens.ScopeEventsRead, apitokens.ScopeAuditRead}} {
		c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, scopes...))
		_, err := c.ExportMessages(context.Background(), externalapi.ExportMessagesParams{})
		testhelper.ExternalRefusal(t, err, http.StatusForbidden, "scopes %v are refused", scopes)
	}
}

func TestExportMessagesAnswers404WithoutTheLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	c := exportClient(t, env, fixtures.InitechID)

	_, err := c.ExportMessages(context.Background(), externalapi.ExportMessagesParams{})

	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

func TestExportMessagesNeverReadsAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	globex := readExportMessages(t, exportClient(t, env, fixtures.GlobexID), "")
	initech := readExportMessages(t, exportClient(t, env, fixtures.InitechID), "")

	require.NotNil(t, globex.row(fixtures.OutboundMessageGlobexID))
	assert.Nil(t, initech.row(fixtures.OutboundMessageGlobexID))
	assert.Nil(t, globex.row(fixtures.OutboundMessageErasableBroadcastID))
}

func TestExportMessagesRefuseAMalformedCursor(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.InitechID)

	_, err := c.ExportMessages(context.Background(), externalapi.ExportMessagesParams{
		Cursor: externalapi.NewOptString("not-a-cursor"),
	})

	testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
}

// A skipped message carries its Skipped reason and a failed one its failure text, each null on the
// other (ADR 0178).
func TestExportMessagesCarriesTheSkippedReasonAndTheFailureApart(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.AcmeID)

	page := readExportMessages(t, c, "")

	skipped := page.row(fixtures.OutboundMessageSkippedID)
	require.NotNil(t, skipped)
	assert.Equal(t, "skipped", skipped["status"])
	assert.Equal(t, "suppressed", skipped["skipped_reason"])
	assert.Contains(t, skipped, "failure")
	assert.Nil(t, skipped["failure"])
	failed := page.row(fixtures.OutboundMessageFailedID)
	require.NotNil(t, failed)
	assert.Equal(t, "failed", failed["status"])
	assert.Equal(t, "provider timeout", failed["failure"])
	assert.Contains(t, failed, "skipped_reason")
	assert.Nil(t, failed["skipped_reason"])
	assert.NotContains(t, failed, "reason")
}

// A message row says whether the message went out tracked and when its tracking was withdrawn
// (ADR 0120): Umbrella's tracking mix Broadcast has one tracked message, one withdrawn since and
// one sent untracked.
func TestExportMessagesCarryWhetherAMessageIsTracked(t *testing.T) {
	env := testhelper.Setup(t)
	c := exportClient(t, env, fixtures.UmbrellaID)

	page := readExportMessages(t, c, "")

	tracked := page.row(fixtures.OutboundMessageUmbrellaTrackedID)
	withdrawn := page.row(fixtures.OutboundMessageUmbrellaWithdrawnID)
	untracked := page.row(fixtures.OutboundMessageUmbrellaUntrackedID)
	require.NotNil(t, tracked)
	require.NotNil(t, withdrawn)
	require.NotNil(t, untracked)
	assert.Equal(t, true, tracked["tracked"])
	assert.Nil(t, tracked["tracking_withdrawn_at"])
	assert.Equal(t, true, withdrawn["tracked"], "it went out tracked")
	assert.NotNil(t, withdrawn["tracking_withdrawn_at"])
	assert.Equal(t, false, untracked["tracked"])
	assert.Nil(t, untracked["tracking_withdrawn_at"])
}

// A message no render decided is neither tracked nor untracked (ADR 0120): one skipped before its
// render, and one still pending, read `tracked` as null.
func TestExportMessagesReadAMessageNoRenderDecidedAsNeither(t *testing.T) {
	env := testhelper.Setup(t)

	acme := readExportMessages(t, exportClient(t, env, fixtures.AcmeID), "")
	globex := readExportMessages(t, exportClient(t, env, fixtures.GlobexID), "")

	skipped := acme.row(fixtures.OutboundMessageSkippedID)
	pending := globex.row(fixtures.OutboundMessageGlobexID)
	require.NotNil(t, skipped)
	require.NotNil(t, pending)
	assert.Nil(t, skipped["tracked"])
	assert.Nil(t, pending["tracked"])
}

// A refusal's reach-back withdraws the tracking of the Destination's message (ADR 0120), and the
// stream delivers the message again with the moment of the withdrawal.
func TestExportMessagesDeliversAWithdrawnMessageAgain(t *testing.T) {
	env := testhelper.Setup(t)
	const usage = 901 // Globex's one Broadcast message in every analytics window, to usage@globex.test
	c := exportClient(t, env, fixtures.GlobexID)
	before := readExportMessages(t, c, "")
	require.Nil(t, before.row(usage)["tracking_withdrawn_at"])

	api := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, apitokens.ScopeContactsWrite))
	_, err := api.EngagementTrackingCreate(t.Context(), &externalapi.RecordEngagementTrackingInput{
		Channel: externalapi.EngagementTrackingChannelEmail, Destination: "usage@globex.test", State: externalapi.EngagementTrackingStateDenied,
	})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})

	after := readExportMessages(t, c, before.cursor)
	withdrawn := after.row(usage)
	require.NotNil(t, withdrawn, "the withdrawn message comes again from the cursor taken before")
	assert.NotNil(t, withdrawn["tracking_withdrawn_at"])
}

// The withdrawal of a default turned to dont_track (ADR 0120) delivers the messages it reached
// again, with the moment their tracking was withdrawn.
func TestExportMessagesDeliversAMessageWithdrawnByTheDefaultAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := exportClient(t, env, fixtures.GlobexID)
	before := readExportMessages(t, c, "")
	require.Nil(t, before.row(fixtures.OutboundMessageGlobexUsageID)["tracking_withdrawn_at"])

	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	_, err := jane.SiteWorkspacesSetEngagementTrackingDefault(ctx, &siteapi.SiteSetEngagementTrackingDefaultInput{
		Revision:                  siteapi.Revision(env.WorkspaceRevision(ctx, t, fixtures.GlobexPublicID)),
		EngagementTrackingDefault: siteapi.EngagementTrackingDecisionDontTrack,
	}, siteapi.SiteWorkspacesSetEngagementTrackingDefaultParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, outboundmessages.WithdrawUngrantedArgs{})

	after := readExportMessages(t, c, before.cursor)
	withdrawn := after.row(fixtures.OutboundMessageGlobexUsageID)
	require.NotNil(t, withdrawn, "the withdrawn row comes again from the cursor taken before")
	assert.Equal(t, true, withdrawn["tracked"], "it went out tracked")
	assert.NotNil(t, withdrawn["tracking_withdrawn_at"])
}

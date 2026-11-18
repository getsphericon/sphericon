package outboundmessages_test

import (
	"testing"
	"time"

	entgo "entgo.io/ent"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Skipped send records why in its Skipped reason and a Failed one what it failed with in its
// failure, each on its own message only (ADR 0178).
func TestASkippedSendRecordsItsReasonAndAFailedOneItsFailure(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	m := newModule(env, time.Now)
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.Send(ctx, s, transactional("tx:skip", suppressd))
	require.NoError(t, err)
	broken := transactional("tx:broken", "a@example.com")
	broken.Content = outboundmessages.EmailContent{Subject: "{% if %}broken", Body: htmlBody}
	_, err = m.Send(ctx, s, broken)
	require.NoError(t, err)

	skipped := byKey(t, env, "tx:skip")
	assert.Equal(t, outboundmessage.StatusSkipped, skipped.Status)
	assert.Equal(t, skipreason.Suppressed, lo.FromPtr(skipped.SkippedReason))
	assert.Nil(t, skipped.Failure)
	failed := byKey(t, env, "tx:broken")
	assert.Equal(t, outboundmessage.StatusFailed, failed.Status)
	assert.NotEmpty(t, lo.FromPtr(failed.Failure))
	assert.Nil(t, failed.SkippedReason)
}

// The Outbound message's Skipped reason declares exactly the closed list of Skipped reasons.
func TestTheSkippedReasonColumnIsEverySkippedReason(t *testing.T) {
	field, ok := lo.Find(schema.OutboundMessage{}.Fields(), func(f entgo.Field) bool {
		return f.Descriptor().Name == outboundmessage.FieldSkippedReason
	})
	require.True(t, ok)
	declared := lo.Map(field.Descriptor().Enums, func(e struct{ N, V string }, _ int) string { return e.V })

	assert.ElementsMatch(t, skipreason.Reason("").Values(), declared)
}

func TestASkippedReasonOutsideTheListIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageStaleClaimID).
		SetStatus(outboundmessage.StatusSkipped).SetSkippedReason("contact unsubscribed").Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsValidationError(err), "got %v", err)
}

func TestTheDatabaseRefusesASkippedMessageWithoutItsReason(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageStaleClaimID).
		SetStatus(outboundmessage.StatusSkipped).Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "got %v", err)
}

func TestTheDatabaseRefusesAFailureOnAMessageThatDidNotFail(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageWelcomeID).
		SetFailure("550 mailbox unavailable").Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "got %v", err)
}

func TestTheDatabaseRefusesASkippedReasonOnAMessageThatWasNotSkipped(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.DB.OutboundMessage.UpdateOneID(fixtures.OutboundMessageWelcomeID).
		SetSkippedReason(skipreason.Suppressed).Exec(t.Context())

	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "got %v", err)
}

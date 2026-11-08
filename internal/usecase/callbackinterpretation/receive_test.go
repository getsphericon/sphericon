package callbackinterpretation_test

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"testing"
	"time"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/callbackinterpretation"
)

// receiveClock is the instant the hook believes it is, and the one callbacks are signed at.
var receiveClock = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// secretGenericA is the secret of the generic Integration IntegrationAcmeGenericA in
// fixtures/integrations.yml: whsec_ and the base64 of a plain string, as the fixture spells it.
var secretGenericA = "whsec_" + base64.StdEncoding.EncodeToString([]byte("sphericon-test-secret-A0123456789"))

func receiver(env *testhelper.TestEnv) *callbackinterpretation.UseCase {
	return callbackinterpretation.New(env.DB, env.Bus, env.Integrations, func() time.Time { return receiveClock })
}

// bounce is a callback of the generic Preset reporting a hard bounce of dest, signed with secret.
func bounce(t *testing.T, key, secret, dest string) callbackinterpretation.Callback {
	t.Helper()
	body := []byte(`{"id":"evt_1","destination":"` + dest + `","kind":"bounce","hard":true,"sending_domain":"Codebasics.dev"}`)
	wh, err := standardwebhooks.NewWebhook(secret)
	require.NoError(t, err)
	sig, err := wh.Sign("msg_1", receiveClock, body)
	require.NoError(t, err)
	header := http.Header{}
	header.Set("Webhook-Id", "msg_1")
	header.Set("Webhook-Timestamp", strconv.FormatInt(receiveClock.Unix(), 10))
	header.Set("Webhook-Signature", sig)
	return callbackinterpretation.Callback{Key: key, Header: header, Body: body}
}

func TestACallbackIsInterpretedInTheWorkspaceItsIngestKeyNames(t *testing.T) {
	env := testhelper.Setup(t)

	err := receiver(env).Receive(t.Context(), bounce(t, fixtures.IntegrationAcmeGenericAIngestKey, secretGenericA, fixtures.ContactAliceEmail))

	require.NoError(t, err)
	got := env.QueuedEnvelopes(t, events.NameEmailBounced)
	require.Len(t, got, 1)
	assert.Equal(t, int64(fixtures.AcmeID), got[0].WorkspaceID)
}

func TestACallbackToAnUnknownKeyHasNoHook(t *testing.T) {
	env := testhelper.Setup(t)

	err := receiver(env).Receive(t.Context(), bounce(t, "omik_does_not_exist", secretGenericA, fixtures.ContactAliceEmail))

	require.ErrorIs(t, err, callbackinterpretation.ErrNoHook)
}

// A Workspace in its deletion window takes no new data (ADR 0043).
func TestACallbackToADeletedWorkspacesKeyHasNoHook(t *testing.T) {
	env := testhelper.Setup(t)
	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(receiveClock).ExecX(t.Context())

	err := receiver(env).Receive(t.Context(), bounce(t, fixtures.IntegrationAcmeGenericAIngestKey, secretGenericA, fixtures.ContactAliceEmail))

	require.ErrorIs(t, err, callbackinterpretation.ErrNoHook)
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameEmailBounced))
}

// An Integration accepts only its own secret: the Preset's refusal comes back as it is.
func TestACallbackSignedForAnotherIntegrationIsNotAuthentic(t *testing.T) {
	env := testhelper.Setup(t)

	err := receiver(env).Receive(t.Context(), bounce(t, fixtures.IntegrationAcmeGenericBIngestKey, secretGenericA, fixtures.ContactAliceEmail))

	require.ErrorIs(t, err, preset.ErrUnauthentic)
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameEmailBounced))
}

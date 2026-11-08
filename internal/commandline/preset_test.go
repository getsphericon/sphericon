package commandline_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
)

func TestWritePresetTestsPrintsPassForEachCase(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WritePresetTests(&out, "header-hmac", []preset.CaseResult{
		{Name: "a signed request passes"}, {Name: "a signature over another body is refused"},
	})

	require.NoError(t, err)
	assert.Equal(t, "PASS a signed request passes\nPASS a signature over another body is refused\n", out.String())
}

func TestWritePresetTestsFailsAndNamesTheCaseThatDisagrees(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WritePresetTests(&out, "broken", []preset.CaseResult{
		{Name: "a signed request passes"},
		{Name: "a signature over another body is refused", Err: errors.New("got notices, want a refusal")},
	})

	require.EqualError(t, err, "preset broken: 1 of 2 cases failed")
	assert.Contains(t, out.String(), "FAIL a signature over another body is refused\n     got notices, want a refusal\n")
	assert.Contains(t, out.String(), "PASS a signed request passes")
}

func TestWritePresetSamplePrintsTheDeliveryNotices(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WritePresetSample(&out, preset.Result{Notices: []preset.DeliveryNotice{{
		ID: "evt_1", Channel: "email", Destination: "alice@example.com", Kind: preset.Bounce, Hard: true, SendingDomain: "news.example.com",
	}}}, nil)

	require.NoError(t, err)
	assert.Equal(t, `bounce hard alice@example.com id=evt_1 sending_domain=news.example.com message_id= reason="" channel=email status= error_code=
`, out.String())
}

func TestWritePresetSamplePrintsInboundMessagesAndReachabilityChanges(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WritePresetSample(&out, preset.Result{
		InboundMessages: []preset.InboundMessage{
			{ID: "7", Channel: "telegram", Destination: "42", ChatID: "43", MessageID: "11", Text: "/stop"},
			{ID: "8", Channel: "telegram", Destination: "42", ChatID: "43", MessageID: "12", Payload: "unsubscribe"},
		},
		ReachabilityChanges: []preset.ReachabilityChange{
			{ID: "9", Destination: "42", Reason: preset.Blocked},
			{ID: "10", Destination: "44", Reachable: true},
		},
	}, nil)

	require.NoError(t, err)
	assert.Equal(t, `inbound_message telegram 42 id=7 chat_id=43 message_id=11 text="/stop" payload=""
inbound_message telegram 42 id=8 chat_id=43 message_id=12 text="" payload="unsubscribe"
reachability_change 42 id=9 unreachable reason=blocked
reachability_change 44 id=10 reachable
`, out.String())
}

func TestWritePresetSampleSaysWhenAVerifiedRequestCarriesNoFacts(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WritePresetSample(&out, preset.Result{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "verified, no facts (an event the Preset does not handle)\n", out.String())
}

func TestWritePresetSamplePrintsTheReasonOfARefusal(t *testing.T) {
	refused := errors.New("preset: the request is not authentic")
	var out bytes.Buffer

	err := commandline.WritePresetSample(&out, preset.Result{}, refused)

	require.ErrorIs(t, err, refused)
	assert.Equal(t, "refused: preset: the request is not authentic\n", out.String())
}

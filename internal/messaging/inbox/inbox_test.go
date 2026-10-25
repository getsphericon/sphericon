package inbox_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/inbox"
)

type recordingCapturer struct{ got []messaging.Captured }

func (r *recordingCapturer) Capture(_ context.Context, c messaging.Captured) error {
	r.got = append(r.got, c)
	return nil
}

func build(t *testing.T, capturer messaging.Capturer) messaging.EmailSender {
	t.Helper()
	catalog := messaging.NewCatalog(inbox.Descriptor())
	sender, err := catalog.BuildEmail(messaging.ProviderInbox, []byte("{}"), messaging.Deps{Capturer: capturer})
	require.NoError(t, err)
	return sender
}

func TestSendKeepsTheMessageWithItsHeadersAndLinksAndConfirmsDelivery(t *testing.T) {
	capturer := &recordingCapturer{}
	sender := build(t, capturer)

	receipt, err := sender.Send(context.Background(), messaging.EmailMessage{
		From: "news@example.com", FromName: "News", To: "ada@example.com", Subject: "Hello",
		HTML:               `<p><a href="https://example.com/a">a</a> <a href="https://example.com/b">b</a> <a href="https://example.com/a">again</a></p>`,
		Text:               "Hello",
		ListUnsubscribeURL: "https://example.com/u/1",
	})
	require.NoError(t, err)

	assert.True(t, receipt.DeliveryConfirmed)
	require.Len(t, capturer.got, 1)
	kept := capturer.got[0]
	assert.Equal(t, receipt.MessageID, kept.MessageID)
	assert.Equal(t, []string{"https://example.com/a", "https://example.com/b"}, kept.Links, "distinct, in order")
	names := lo.Map(kept.Headers, func(h messaging.Header, _ int) string { return h.Name })
	assert.Contains(t, names, "From")
	assert.Contains(t, names, "To")
	assert.Contains(t, names, "Subject")
	assert.Contains(t, names, "Message-ID")
	assert.Contains(t, names, "List-Unsubscribe")
	assert.Contains(t, names, "List-Unsubscribe-Post")
}

func TestTheInboxDeliversNowhereOutsideAndReportsItsOwnCeiling(t *testing.T) {
	sender := build(t, &recordingCapturer{})

	assert.False(t, messaging.DeliversOutside(sender))
	reader, ok := messaging.AsQuotaReader(sender)
	require.True(t, ok)
	quota, err := reader.SendQuota(context.Background())
	require.NoError(t, err)
	require.NotNil(t, quota.PerDay)
	assert.Equal(t, inbox.PerDay, *quota.PerDay)
	assert.Nil(t, quota.PerSecond)
}

func TestSendWithoutAPlaceToKeepTheMessageFails(t *testing.T) {
	sender := build(t, nil)

	_, err := sender.Send(context.Background(), messaging.EmailMessage{From: "a@example.com", To: "b@example.com", Subject: "x", Text: "x"})

	require.ErrorIs(t, err, inbox.ErrNoCapturer)
}

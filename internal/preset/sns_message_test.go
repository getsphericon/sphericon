package preset

import (
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AWS signs the fields a message type carries, name then value on lines of their own, in
// alphabetical order; a Notification's Subject only when it has one.
func TestSNSStringToSignFollowsTheMessageType(t *testing.T) {
	tests := []struct {
		name string
		msg  snsMessage
		want string
	}{
		{
			name: "a notification with a subject",
			msg:  snsMessage{Type: "Notification", Message: "m", MessageID: "id", Subject: "s", Timestamp: "t", TopicArn: "arn", SubscribeURL: "ignored", Token: "ignored"},
			want: "Message\nm\nMessageId\nid\nSubject\ns\nTimestamp\nt\nTopicArn\narn\nType\nNotification\n",
		},
		{
			name: "a notification without one",
			msg:  snsMessage{Type: "Notification", Message: "m", MessageID: "id", Timestamp: "t", TopicArn: "arn"},
			want: "Message\nm\nMessageId\nid\nTimestamp\nt\nTopicArn\narn\nType\nNotification\n",
		},
		{
			name: "a subscription confirmation",
			msg:  snsMessage{Type: "SubscriptionConfirmation", Message: "m", MessageID: "id", SubscribeURL: "url", Timestamp: "t", Token: "tok", TopicArn: "arn", Subject: "ignored"},
			want: "Message\nm\nMessageId\nid\nSubscribeURL\nurl\nTimestamp\nt\nToken\ntok\nTopicArn\narn\nType\nSubscriptionConfirmation\n",
		},
		{
			name: "an unsubscribe confirmation",
			msg:  snsMessage{Type: "UnsubscribeConfirmation", Message: "m", MessageID: "id", SubscribeURL: "url", Timestamp: "t", Token: "tok", TopicArn: "arn"},
			want: "Message\nm\nMessageId\nid\nSubscribeURL\nurl\nTimestamp\nt\nToken\ntok\nTopicArn\narn\nType\nUnsubscribeConfirmation\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.msg.stringToSign()

			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestSNSStringToSignRefusesAnUnknownMessageType(t *testing.T) {
	_, err := (&snsMessage{Type: "Telegram"}).stringToSign()

	require.ErrorContains(t, err, "unknown")
}

func TestSNSSignatureVersionPicksTheAlgorithm(t *testing.T) {
	assert.Equal(t, x509.SHA1WithRSA, (&snsMessage{SignatureVersion: "1"}).algorithm())
	assert.Equal(t, x509.SHA256WithRSA, (&snsMessage{SignatureVersion: "2"}).algorithm())
}

package preset_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/preset"
)

// The Presets' own cases are signed with a certificate whose key was discarded, so they cannot
// sign a new message. These tests sign with a key of their own and hand the Preset its
// certificate, to reach what those samples do not: each message type's fields and both
// signature versions.

const (
	snsTopic   = "arn:aws:sns:eu-west-1:123456789012:acme-ses-events"
	snsCertURL = "https://sns.eu-west-1.amazonaws.com/SimpleNotificationService-test.pem"
)

const snsProbe = `
version: 1
name: sns-probe
params: {type: object}
verify: {kind: sns, topic: topic_arn}
transform: "[]"
tests:
  - {name: unsigned, request: {body: "{}"}, expect: {refused: unauthentic}}
`

// snsSigner is an AWS stand-in: a key and the certificate an `sns` Preset is given for it.
type snsSigner struct {
	key    *rsa.PrivateKey
	preset *preset.Preset
}

func newSNSSigner(t *testing.T) *snsSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sns.amazonaws.com"},
		NotBefore: time.Unix(0, 0), NotAfter: time.Unix(1<<32, 0)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	p, err := preset.Parse("sns-probe.yaml", []byte(snsProbe),
		preset.WithSNSCertificates(func(context.Context, string) (*x509.Certificate, error) { return cert, nil }))
	require.NoError(t, err)
	return &snsSigner{key: key, preset: p}
}

// handle signs stringToSign with hash, puts the signature into msg under version and hands
// the message to the Preset.
func (s *snsSigner) handle(t *testing.T, msg map[string]string, version string, hash crypto.Hash, stringToSign string) error {
	t.Helper()
	h := hash.New()
	h.Write([]byte(stringToSign))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, hash, h.Sum(nil))
	require.NoError(t, err)
	msg["Signature"] = base64.StdEncoding.EncodeToString(sig)
	msg["SignatureVersion"] = version
	msg["SigningCertURL"] = snsCertURL
	body, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = s.preset.Handle(context.Background(), preset.Request{Body: body}, preset.Params{"topic_arn": snsTopic}, time.Now())
	return err
}

// AWS signs the fields a message type carries, name then value on lines of their own, in
// alphabetical order; a Notification's Subject only when it has one. A field the type does
// not carry is not signed, whatever the body holds.
func TestSNSVerifiesTheFieldsEachMessageTypeSigns(t *testing.T) {
	signer := newSNSSigner(t)
	tests := []struct {
		name         string
		msg          map[string]string
		stringToSign string
	}{
		{
			name: "a notification with a subject",
			msg: map[string]string{"Type": "Notification", "Message": "m", "MessageId": "id", "Subject": "s", "Timestamp": "t",
				"TopicArn": snsTopic, "SubscribeURL": "ignored", "Token": "ignored"},
			stringToSign: "Message\nm\nMessageId\nid\nSubject\ns\nTimestamp\nt\nTopicArn\n" + snsTopic + "\nType\nNotification\n",
		},
		{
			name:         "a notification without one",
			msg:          map[string]string{"Type": "Notification", "Message": "m", "MessageId": "id", "Timestamp": "t", "TopicArn": snsTopic},
			stringToSign: "Message\nm\nMessageId\nid\nTimestamp\nt\nTopicArn\n" + snsTopic + "\nType\nNotification\n",
		},
		{
			name: "a subscription confirmation",
			msg: map[string]string{"Type": "SubscriptionConfirmation", "Message": "m", "MessageId": "id", "SubscribeURL": snsCertURL,
				"Timestamp": "t", "Token": "tok", "TopicArn": snsTopic, "Subject": "ignored"},
			stringToSign: "Message\nm\nMessageId\nid\nSubscribeURL\n" + snsCertURL + "\nTimestamp\nt\nToken\ntok\nTopicArn\n" + snsTopic + "\nType\nSubscriptionConfirmation\n",
		},
		{
			name: "an unsubscribe confirmation",
			msg: map[string]string{"Type": "UnsubscribeConfirmation", "Message": "m", "MessageId": "id", "SubscribeURL": snsCertURL,
				"Timestamp": "t", "Token": "tok", "TopicArn": snsTopic},
			stringToSign: "Message\nm\nMessageId\nid\nSubscribeURL\n" + snsCertURL + "\nTimestamp\nt\nToken\ntok\nTopicArn\n" + snsTopic + "\nType\nUnsubscribeConfirmation\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, signer.handle(t, tc.msg, "2", crypto.SHA256, tc.stringToSign))
		})
	}
}

func TestSNSRefusesAnUnknownMessageType(t *testing.T) {
	signer := newSNSSigner(t)
	msg := map[string]string{"Type": "Telegram", "Message": "m", "MessageId": "id", "Timestamp": "t", "TopicArn": snsTopic}

	err := signer.handle(t, msg, "2", crypto.SHA256, "Message\nm\nMessageId\nid\nTimestamp\nt\nTopicArn\n"+snsTopic+"\nType\nTelegram\n")

	require.ErrorIs(t, err, preset.ErrUnauthentic)
}

// Signature version 1 is SHA-1 and version 2 SHA-256, both over RSA: a message is checked with
// the hash its version names.
func TestSNSSignatureVersionPicksTheHash(t *testing.T) {
	signer := newSNSSigner(t)
	notification := func() map[string]string {
		return map[string]string{"Type": "Notification", "Message": "m", "MessageId": "id", "Timestamp": "t", "TopicArn": snsTopic}
	}
	signed := "Message\nm\nMessageId\nid\nTimestamp\nt\nTopicArn\n" + snsTopic + "\nType\nNotification\n"

	require.NoError(t, signer.handle(t, notification(), "1", crypto.SHA1, signed), "version 1 over SHA-1")
	require.NoError(t, signer.handle(t, notification(), "2", crypto.SHA256, signed), "version 2 over SHA-256")
	require.ErrorIs(t, signer.handle(t, notification(), "1", crypto.SHA256, signed), preset.ErrUnauthentic, "version 1 over SHA-256")
	require.ErrorIs(t, signer.handle(t, notification(), "2", crypto.SHA1, signed), preset.ErrUnauthentic, "version 2 over SHA-1")
}

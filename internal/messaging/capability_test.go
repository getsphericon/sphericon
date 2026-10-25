package messaging_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/messaging"
)

type silentSender struct{}

func (silentSender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{}, nil
}

type keepingSender struct{ silentSender }

func (keepingSender) DeliversOutside() bool { return false }

type recordingCapturer struct{ got []messaging.EmailMessage }

func (r *recordingCapturer) Capture(_ context.Context, c messaging.Captured) error {
	r.got = append(r.got, c.Message)
	return nil
}

// A sender says whether it delivers outside; one that says nothing does, and the answer
// survives the metrics decorator the catalog wraps every sender in.
func TestDeliversOutsideIsReadThroughTheMetricsDecorator(t *testing.T) {
	catalog := messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.ProviderSMTP,
			Build: func([]byte, messaging.Deps) (any, error) { return silentSender{}, nil },
		},
		messaging.ProviderDescriptor{
			Channel: messaging.ChannelEmail, Provider: messaging.ProviderSES,
			Build: func([]byte, messaging.Deps) (any, error) { return keepingSender{}, nil },
		},
	)

	silent, err := catalog.BuildEmail(messaging.ProviderSMTP, nil, messaging.Deps{})
	require.NoError(t, err)
	kept, err := catalog.BuildEmail(messaging.ProviderSES, nil, messaging.Deps{})
	require.NoError(t, err)

	assert.True(t, messaging.DeliversOutside(silent), "a sender that says nothing delivers outside")
	assert.False(t, messaging.DeliversOutside(kept))
}

// A builder receives the dependencies the catalog is given, the signer and the capturer
// bound to the Workspace's scope.
func TestBuilderReceivesItsDependencies(t *testing.T) {
	capturer := &recordingCapturer{}
	var got messaging.Deps
	catalog := messaging.NewCatalog(messaging.ProviderDescriptor{
		Channel: messaging.ChannelEmail, Provider: messaging.ProviderSMTP,
		Build: func(_ []byte, deps messaging.Deps) (any, error) {
			got = deps
			return silentSender{}, nil
		},
	})

	_, err := catalog.BuildEmail(messaging.ProviderSMTP, nil, messaging.Deps{Capturer: capturer})
	require.NoError(t, err)

	require.NoError(t, got.Capturer.Capture(context.Background(), messaging.Captured{Message: messaging.EmailMessage{Subject: "kept"}}))
	require.Len(t, capturer.got, 1)
	assert.Equal(t, "kept", capturer.got[0].Subject)
}

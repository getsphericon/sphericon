package outbound_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Sending-domain gate keeps applying to the providers that deliver outside exactly as before
// the Test inbox existed (ADR 0058): built by the real descriptors, with no verified Sending
// domain (Globex has none), a send through either is Held and nothing leaves.
func TestRealProvidersWithoutAVerifiedSendingDomainAreHeld(t *testing.T) {
	catalog := messaging.NewCatalog(ses.Descriptor(), smtp.Descriptor())
	cases := map[string]struct {
		provider messaging.Provider
		config   string
	}{
		"SES":  {messaging.ProviderSES, `{"region":"eu-west-1","accessKeyId":"AKIATEST","secretAccessKey":"s","from":"hello@globex.example"}`},
		"SMTP": {messaging.ProviderSMTP, `{"host":"smtp.example.com","port":587,"from":"hello@globex.example"}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			sender, err := catalog.BuildEmail(c.provider, []byte(c.config), messaging.Deps{})
			require.NoError(t, err)
			require.True(t, messaging.DeliversOutside(sender), "a real provider delivers outside")

			res, err := moduleThrough(env, sender).Send(context.Background(), env.DB.Scoped(fixtures.GlobexID), transactional("gate:"+name, "a@example.com"))

			require.NoError(t, err)
			assert.Equal(t, outbound.Held, res.Outcome)
			assert.Equal(t, outbound.HoldUnverifiedDomain, res.Reason)
		})
	}
}

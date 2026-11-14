package auditentries_test

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

type recorder struct{ names []string }

func (r *recorder) Dispatch(_ context.Context, _ *ent.Scoped, d events.WebhookDelivery) error {
	r.names = append(r.names, d.Name)
	return nil
}

func licenseFor(t *testing.T, features ...licensekey.Feature) *licensekey.License {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, nil, features...)
	require.NoError(t, err)
	lic, err := licensekey.Parse(key, pub, time.Now())
	require.NoError(t, err)
	return lic
}

func TestForwarderDeliversAuditEntriesOnlyUnderLicense(t *testing.T) {
	unlicensed := licenseFor(t)

	next := &recorder{}
	require.NoError(t, auditentries.Forwarder(next, unlicensed).Dispatch(t.Context(), nil, events.WebhookDelivery{Name: events.NameAuditEntry, ID: "d1"}))
	assert.Empty(t, next.names, "without a license nothing is delivered")
	require.NoError(t, auditentries.Forwarder(next, unlicensed).Dispatch(t.Context(), nil, events.WebhookDelivery{Name: systemevent.ContactCreated, ID: "d2"}))
	assert.Equal(t, []string{systemevent.ContactCreated}, next.names, "customer events are not gated")

	next = &recorder{}
	require.NoError(t, auditentries.Forwarder(next, licenseFor(t, licensekey.FeatureAudit)).Dispatch(t.Context(), nil, events.WebhookDelivery{Name: events.NameAuditEntry, ID: "d3"}))
	assert.Equal(t, []string{events.NameAuditEntry}, next.names)
}

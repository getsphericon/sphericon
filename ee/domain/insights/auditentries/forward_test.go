package auditentries_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type recorder struct{ names []string }

func (r *recorder) Dispatch(_ context.Context, _ *ent.Scoped, d events.WebhookDelivery) error {
	r.names = append(r.names, d.Name)
	return nil
}

func TestForwarderDeliversAuditEntriesOnlyUnderLicense(t *testing.T) {
	unlicensed := testhelper.License(t)

	next := &recorder{}
	require.NoError(t, auditentries.Forwarder(next, unlicensed).Dispatch(t.Context(), nil, events.WebhookDelivery{Name: events.NameAuditEntry, ID: "d1"}))
	assert.Empty(t, next.names, "without a license nothing is delivered")
	require.NoError(t, auditentries.Forwarder(next, unlicensed).Dispatch(t.Context(), nil, events.WebhookDelivery{Name: systemevent.ContactCreated, ID: "d2"}))
	assert.Equal(t, []string{systemevent.ContactCreated}, next.names, "customer events are not gated")

	next = &recorder{}
	require.NoError(t, auditentries.Forwarder(next, testhelper.License(t, licensekey.FeatureAudit)).Dispatch(t.Context(), nil, events.WebhookDelivery{Name: events.NameAuditEntry, ID: "d3"}))
	assert.Equal(t, []string{events.NameAuditEntry}, next.names)
}

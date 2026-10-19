package site_test

import (
	"crypto/ed25519"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/licensekey"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// lapse is a key pair and the keys it signs for the lapse scenarios (ADR 0035): one that
// expired yesterday, one that is still valid, and one signed by somebody else.
type lapse struct {
	pub                    ed25519.PublicKey
	expired, valid, forged string
	expiredOn              time.Time
}

func newLapse(t *testing.T) lapse {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, other, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	l := lapse{pub: pub, expiredOn: time.Now().Add(-24 * time.Hour).Truncate(time.Second)}
	future := time.Now().Add(24 * time.Hour)
	features := []licensekey.Feature{licensekey.FeatureAudit, licensekey.FeatureRetention, licensekey.FeatureStaff}
	l.expired, err = licensekey.Issue(priv, &l.expiredOn, features...)
	require.NoError(t, err)
	l.valid, err = licensekey.Issue(priv, &future, features...)
	require.NoError(t, err)
	l.forged, err = licensekey.Issue(other, &future, features...)
	require.NoError(t, err)
	return l
}

func TestAnExpiredLicenseKeyBootsAsTheCoreAndTheOwnerSeesWhen(t *testing.T) {
	l := newLapse(t)
	env := testhelper.Setup(t, testhelper.WithLicenseKey(l.pub, l.expired))
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	state := instanceState(t, john)
	require.True(t, state.LicenseExpiredAt.Set)
	assert.True(t, time.Time(state.LicenseExpiredAt.Value).Equal(l.expiredOn))
	_, listed := auditList(t, john, fixtures.AcmeSlug).(*siteapi.SiteAuditEntryList)
	assert.False(t, listed, "the Audit log is dark without its feature")
	report, err := env.Instance.Report(t.Context())
	require.NoError(t, err)
	assert.Equal(t, telemetryapi.TelemetryEditionCore, report.Edition, "a lapsed key reports the core")
	requireInstanceForbidden(t, env.SiteActor(t, fixtures.MemberMaryEmail))
}

func TestAValidLicenseKeyShowsNoLapse(t *testing.T) {
	l := newLapse(t)
	env := testhelper.Setup(t, testhelper.WithLicenseKey(l.pub, l.valid))

	assert.False(t, instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).LicenseExpiredAt.Set)
}

func TestAForgedLicenseKeyIsRefusedEvenWhenItIsNotExpired(t *testing.T) {
	l := newLapse(t)

	_, err := licensekey.Load(l.forged, l.pub, time.Now(), slog.Default())

	assert.ErrorIs(t, err, licensekey.ErrInvalid)
}

func TestRenewingTheKeyRestoresTheAuditLogWithItsData(t *testing.T) {
	l := newLapse(t)
	env := testhelper.Setup(t, testhelper.WithLicenseKey(l.pub, l.expired))
	acme := env.DB.Scoped(fixtures.AcmeID)
	rows := acme.AuditEntry().Query().CountX(t.Context())
	require.Positive(t, rows, "the fixtures carry an Audit entry")
	_, listed := auditList(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmeSlug).(*siteapi.SiteAuditEntryList)
	require.False(t, listed, "dark while the key is lapsed")

	// Renewal: the same database read under the renewed key, as after a restart.
	renewed, err := licensekey.Parse(l.valid, l.pub, time.Now())
	require.NoError(t, err)
	log := audit.NewLog(renewed)
	require.True(t, log.Licensed())
	entries, _, err := log.Entries(t.Context(), acme, events.AuditFilter{}, 0, rows+1)
	require.NoError(t, err)
	assert.Len(t, entries, rows, "nothing was deleted or altered while lapsed")
}

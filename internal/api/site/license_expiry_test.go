package site_test

import (
	"crypto/ed25519"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/licensekey"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// lapse is a key pair and the keys it signs for the lapse scenarios (ADR 0035): one that
// expired yesterday, one that is still valid, and one signed by somebody else.
type lapse struct {
	pub                    ed25519.PublicKey
	expired, valid, forged string
	expiredOn              time.Time
}

// lapseNow is the frozen clock of the lapse scenarios, so the keys' dates never drift.
var lapseNow = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)

func lapseClock() time.Time { return lapseNow }

func newLapse(t *testing.T) lapse {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, other, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	l := lapse{pub: pub, expiredOn: lapseNow.Add(-24 * time.Hour)}
	future := lapseNow.Add(24 * time.Hour)
	features := []licensekey.Feature{licensekey.FeatureAudit, licensekey.FeatureRetention, licensekey.FeatureStaff}
	l.expired, err = licensekey.Issue(priv, &l.expiredOn, features...)
	require.NoError(t, err)
	l.valid, err = licensekey.Issue(priv, &future, features...)
	require.NoError(t, err)
	l.forged, err = licensekey.Issue(other, &future, features...)
	require.NoError(t, err)
	return l
}

// The Staff console shows when the key expired (internal/api/staff); no Workspace does.
func TestAnExpiredLicenseKeyBootsAsTheCore(t *testing.T) {
	l := newLapse(t)
	env := testhelper.Setup(t, testhelper.WithLicenseKey(l.pub, l.expired), testhelper.WithClock(lapseClock))
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := john.SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusPaymentRequired, "the Audit log is dark without its feature")
	report, err := env.Instance.TelemetryReport(t.Context())
	require.NoError(t, err)

	assert.Equal(t, telemetryapi.TelemetryEditionCore, report.Edition, "a lapsed key reports the core")
}

func TestAForgedLicenseKeyIsRefusedEvenWhenItIsNotExpired(t *testing.T) {
	l := newLapse(t)

	_, err := licensekey.Load(l.forged, l.pub, lapseNow, slog.Default())

	assert.ErrorIs(t, err, licensekey.ErrInvalid)
}

func TestRenewingTheKeyRestoresTheAuditLogWithItsData(t *testing.T) {
	l := newLapse(t)
	env := testhelper.Setup(t, testhelper.WithLicenseKey(l.pub, l.expired), testhelper.WithClock(lapseClock))
	acme := env.DB.Scoped(fixtures.AcmeID)
	rows := acme.AuditEntry().Query().CountX(t.Context())
	require.Positive(t, rows, "the fixtures carry an Audit entry")
	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusPaymentRequired, "dark while the key is lapsed")

	// Renewal: the same database read under the renewed key, as after a restart.
	renewed, err := licensekey.Parse(l.valid, l.pub, lapseNow)
	require.NoError(t, err)
	log := auditentries.NewLog(renewed, env.Bus)
	require.True(t, log.Licensed())
	page, err := log.Entries(t.Context(), acme, events.AuditFilter{}, 0, rows+1)
	require.NoError(t, err)
	assert.Len(t, page.Items, rows, "nothing was deleted or altered while lapsed")
}

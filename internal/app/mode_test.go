package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/cleanup"
)

// saas configures a saas Instance with everything its Mode requires but the license key.
func saas(t *testing.T) {
	t.Helper()
	baseline(t)
	t.Setenv("SPHERICON_MODE", "saas")
	t.Setenv("SPHERICON_STAFF_JWT_SECRET", "staff-secret-for-the-saas-mode-test-0000000000")
	t.Setenv("SPHERICON_INTERNAL_ADDR", freeAddr(t))
	t.Setenv("SPHERICON_TRACKING_CNAME_TARGET", "edge.example.test")
	t.Setenv("SPHERICON_SMTP_HOST", "127.0.0.1")
}

// licenseWith sets a license key with these features, signed by the development key the test
// profile trusts.
func licenseWith(t *testing.T, features ...licensekey.Feature) {
	t.Helper()
	key, err := licensekey.Issue(licensekey.DevPrivateKey(), nil, features...)
	require.NoError(t, err)
	t.Setenv("SPHERICON_LICENSE_KEY", key)
}

func TestNewRefusesASaaSInstanceWithoutALicenseKey(t *testing.T) {
	saas(t)
	t.Setenv("SPHERICON_LICENSE_KEY", "")

	_, err := app.New("test")

	require.ErrorContains(t, err, "staff")
}

func TestNewRefusesASaaSInstanceWhoseKeyLacksStaff(t *testing.T) {
	saas(t)
	licenseWith(t, licensekey.FeatureAudit)

	_, err := app.New("test")

	require.ErrorContains(t, err, "staff")
}

func TestNewStartsASaaSInstanceLicensedForStaff(t *testing.T) {
	saas(t)
	licenseWith(t, licensekey.FeatureStaff)

	a, err := app.New("test")

	require.NoError(t, err)
	cleanup.Shutdown(a.Shutdown(context.Background()))
}

func TestNewStartsASelfHostedInstanceWithoutALicenseKey(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_MODE", "selfhosted")
	t.Setenv("SPHERICON_LICENSE_KEY", "")

	a, err := app.New("test")

	require.NoError(t, err)
	cleanup.Shutdown(a.Shutdown(context.Background()))
}

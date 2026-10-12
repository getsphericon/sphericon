package testhelper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/usage"
)

// RefreshUsage runs the usage job once as of now under the harness license, which
// includes the operator feature: it closes the elapsed Usage periods and refreshes the
// Live usage of the open one, as the periodic job does (the river schedule does not run
// in tests).
func (env *TestEnv) RefreshUsage(t *testing.T, now time.Time) {
	t.Helper()
	lic, err := testLicense()
	require.NoError(t, err)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, now))
}

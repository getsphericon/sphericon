package testhelper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
)

// RefreshUsage runs the usage job once as of now under the harness license, which
// includes the `staff` license feature: it refreshes every Workspace the job lists, closing the
// elapsed Usage periods and refreshing the Live usage of the open one, as the periodic job's
// fan-out does (the river schedule does not run in tests).
func (env *TestEnv) RefreshUsage(t *testing.T, now time.Time) {
	t.Helper()
	lic, err := testLicense()
	require.NoError(t, err)
	m := usages.New(env.DB, lic, func() time.Time { return now })
	due, err := m.Due(t.Context())
	require.NoError(t, err)
	for _, ws := range due {
		require.NoError(t, m.Refresh(t.Context(), ws))
	}
}

package usages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestASendJustBeforeAndJustAfterMidnightUTCLandInDifferentPeriods(t *testing.T) {
	env := testhelper.Setup(t)
	// Meter has a send at 23:59 of month 20 and one at 00:01 of month 19.
	require.NoError(t, usages.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureStaff), month(19).Add(time.Hour)))

	january, february := figure(t, env, month(20)), figure(t, env, month(19))
	assert.EqualValues(t, 1, january.Value)
	assert.True(t, january.Final)
	assert.EqualValues(t, 1, february.Value)
	assert.False(t, february.Final)
}

func TestEveryWorkspaceIsClosedWithinAnHourOfMidnightUTC(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureStaff)
	require.NoError(t, usages.Refresh(t.Context(), env.DB, lic, month(6).Add(-time.Minute)), "the last run of January")

	// The next scheduled tick after midnight is at most one Interval later.
	require.Less(t, usages.Interval, time.Hour, "the job runs several times inside the bound")
	require.NoError(t, usages.Refresh(t.Context(), env.DB, lic, month(6).Add(usages.Interval)))

	page, err := usages.List(t.Context(), env.DB, month(7), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.GreaterOrEqual(t, page.TotalItems, 4, "every fixture Workspace has January figures")
	for _, w := range page.Items {
		require.Len(t, w.Figures, 3, "workspace %d has every metric", w.WorkspaceID)
		for _, f := range w.Figures {
			assert.True(t, f.Final, "workspace %d %s is final", w.WorkspaceID, f.Metric)
		}
	}
}

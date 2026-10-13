package usage_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestASendJustBeforeAndJustAfterMidnightUTCLandInDifferentPeriods(t *testing.T) {
	env := testhelper.Setup(t)
	message(t, env, "before", outboundmessage.KindBroadcast, outboundmessage.StatusSent, time.Date(2027, time.January, 31, 23, 59, 0, 0, time.UTC))
	message(t, env, "after", outboundmessage.KindBroadcast, outboundmessage.StatusSent, time.Date(2027, time.February, 1, 0, 1, 0, 0, time.UTC))

	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureOperator), feb.Add(time.Hour)))

	january, february := figure(t, env, jan), figure(t, env, feb)
	assert.EqualValues(t, 1, january.Value)
	assert.True(t, january.Final)
	assert.EqualValues(t, 1, february.Value)
	assert.False(t, february.Final)
}

func TestEveryWorkspaceIsClosedWithinAnHourOfMidnightUTC(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureOperator)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, feb.Add(-time.Minute)), "the last run of January")

	// The next scheduled tick after midnight is at most one Interval later.
	require.Less(t, usage.Interval, time.Hour, "the job runs several times inside the bound")
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, feb.Add(usage.Interval)))

	page, err := usage.List(t.Context(), env.DB, jan, pagination.Params{Page: 1, PageSize: pagination.MaxPageSize})
	require.NoError(t, err)
	require.GreaterOrEqual(t, page.TotalItems, 4, "every fixture Workspace has January figures")
	for _, w := range page.Items {
		require.Len(t, w.Figures, 3, "workspace %d has every metric", w.WorkspaceID)
		for _, f := range w.Figures {
			assert.True(t, f.Final, "workspace %d %s is final", w.WorkspaceID, f.Metric)
		}
	}
}

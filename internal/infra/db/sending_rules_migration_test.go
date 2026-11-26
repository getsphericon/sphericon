package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/frequencycap"
	"github.com/getsphericon/sphericon/ent/sendingwindow"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Workspace that existed before the sending rules gets the SMS Frequency cap and Sending window
// a new one starts with (ADR 0137): one already texting is protected from the day they ship. A
// deleted one that may be restored gets them too; an erased one, which sends nothing, does not.
func TestTheSendingRulesMigrationGivesEveryWorkspaceTheSMSDefaults(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "sending_rules", filepath.Join("testdata", "sending_rules"))

	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)

	client := poolClient(sqlDB)
	caps, err := client.FrequencyCap.Query().Order(ent.Asc(frequencycap.FieldWorkspaceID)).All(t.Context())
	require.NoError(t, err)
	windows, err := client.SendingWindow.Query().Order(ent.Asc(sendingwindow.FieldWorkspaceID)).All(t.Context())
	require.NoError(t, err)
	type capRow struct {
		workspace           int64
		channel             channels.Channel
		country             *string
		maxMessages, period int
	}
	gotCaps := make([]capRow, 0, len(caps))
	for _, c := range caps {
		require.NotNil(t, c.MaxMessages)
		require.NotNil(t, c.PeriodMinutes)
		gotCaps = append(gotCaps, capRow{c.WorkspaceID, c.Channel, c.Country, *c.MaxMessages, *c.PeriodMinutes})
	}
	assert.Equal(t, []capRow{{1, channels.SMS, nil, 3, 1440}, {2, channels.SMS, nil, 3, 1440}}, gotCaps)
	type windowRow struct {
		workspace      int64
		channel        channels.Channel
		country        *string
		monday, sunday [2]*int
	}
	gotWindows := make([]windowRow, 0, len(windows))
	for _, w := range windows {
		gotWindows = append(gotWindows, windowRow{w.WorkspaceID, w.Channel, w.Country,
			[2]*int{w.MondayOpens, w.MondayCloses}, [2]*int{w.SundayOpens, w.SundayCloses}})
	}
	eight, twenty := 480, 1200
	day := [2]*int{&eight, &twenty}
	assert.Equal(t, []windowRow{{1, channels.SMS, nil, day, day}, {2, channels.SMS, nil, day, day}}, gotWindows)
}

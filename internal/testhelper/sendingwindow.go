package testhelper

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// WindowSMS gives the Workspace an SMS Sending window open every day from opens to closes, each a
// time of the day, in place of its windows, as its settings would (ADR 0134): the fixture windows
// are open all day, so a test of the window narrows the one it reads. Its caps stay as they are.
func (env *TestEnv) WindowSMS(tb testing.TB, workspaceID int64, opens, closes time.Duration) {
	tb.Helper()
	s := env.DB.Scoped(workspaceID)
	m := Module[*workspaces.Module](env)
	rules, err := m.SendingRules(tb.Context(), s)
	require.NoError(tb, err)
	hours := lo.Map(localcalendar.Weekdays, func(day localcalendar.Weekday, _ int) workspaces.OpenHours {
		return workspaces.OpenHours{Day: day, OpensAt: int(opens.Minutes()), ClosesAt: int(closes.Minutes())}
	})
	_, err = m.ReplaceSendingRules(tb.Context(), s, workspaces.ReplaceSendingRulesInput{
		Revision:       rules.Revision,
		FrequencyCaps:  rules.FrequencyCaps,
		SendingWindows: []workspaces.SendingWindow{{Channel: channels.SMS, Hours: hours}},
	})
	require.NoError(tb, err)
}

package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const reminderAppURL = "https://app.example/"

// Umbrella's requirement started 2026-03-01: Rita's grace ends 2026-03-08, Nina's (she
// joined 2026-03-05) 2026-03-12. Sam has a Second factor. Fixture times are UTC.
var (
	ritaDeadline = time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	ninaDeadline = time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)
)

func remind(t *testing.T, env *testhelper.TestEnv, now time.Time) {
	t.Helper()
	require.NoError(t, jobs.RemindSecondFactorDeadlines(context.Background(), env.DB, env.SystemMail, reminderAppURL, now))
}

func recipients(env *testhelper.TestEnv) []string {
	to := make([]string, 0, len(env.SystemMail.Messages()))
	msgs := env.SystemMail.Messages()
	for i := range msgs {
		to = append(to, msgs[i].To)
	}
	return to
}

func TestSecondFactorReminderIsSentOnceADayBeforeTheDeadline(t *testing.T) {
	env := testhelper.Setup(t)

	remind(t, env, ritaDeadline.Add(-25*time.Hour))
	assert.Empty(t, env.SystemMail.Messages(), "more than a day before any deadline")

	remind(t, env, ritaDeadline.Add(-23*time.Hour))
	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1, "only Rita's deadline is a day away")
	assert.Equal(t, fixtures.UmbrellaOwnerRitaEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Subject, "One day left")
	assert.Contains(t, msgs[0].Subject, "Umbrella")
	assert.Contains(t, msgs[0].Text, ritaDeadline.UTC().Format("2006-01-02 15:04 UTC"))
	assert.Contains(t, msgs[0].Text, "https://app.example/account/security")

	remind(t, env, ritaDeadline.Add(-22*time.Hour))
	remind(t, env, ritaDeadline.Add(-time.Hour))
	assert.Len(t, env.SystemMail.Messages(), 1, "Rita is reminded once")

	remind(t, env, ninaDeadline.Add(-12*time.Hour))
	assert.Equal(t, []string{fixtures.UmbrellaOwnerRitaEmail, fixtures.UmbrellaMemberNinaEmail}, recipients(env),
		"Nina's later Membership has its own deadline; Sam, with a Second factor, gets nothing")
}

func TestNoSecondFactorReminderForAUserWhoEnrolledOrWhoseGraceEnded(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.DB.User.UpdateOneID(fixtures.UmbrellaMemberNinaID).
		SetSecondFactorSecretEncrypted("sealed").
		SetSecondFactorConfirmedAt(ninaDeadline.Add(-48 * time.Hour)).
		Save(context.Background())
	require.NoError(t, err)

	remind(t, env, ninaDeadline.Add(-12*time.Hour))
	assert.Empty(t, env.SystemMail.Messages(), "Nina enrolled; Rita's grace has already ended")
}

func TestSecondFactorReminderIsSentAgainForARestartedRequirement(t *testing.T) {
	env := testhelper.Setup(t)
	remind(t, env, ritaDeadline.Add(-12*time.Hour))
	require.Len(t, env.SystemMail.Messages(), 1)

	// Switched off and on again a month later: a new grace for everyone.
	restart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	_, err := env.DB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetSecondFactorRequiredAt(restart).Save(context.Background())
	require.NoError(t, err)

	remind(t, env, restart.Add(6*24*time.Hour+time.Hour))
	to := recipients(env)
	require.Len(t, to, 3)
	assert.ElementsMatch(t, []string{fixtures.UmbrellaOwnerRitaEmail, fixtures.UmbrellaMemberNinaEmail}, to[1:],
		"Rita, already reminded in the earlier grace, is reminded again")
}

func TestAFailedSecondFactorReminderIsRetriedOnTheNextTick(t *testing.T) {
	env := testhelper.Setup(t)
	env.SystemMail.SetErr(errors.New("smtp down"))
	require.Error(t, jobs.RemindSecondFactorDeadlines(context.Background(), env.DB, env.SystemMail, reminderAppURL, ritaDeadline.Add(-12*time.Hour)))

	env.SystemMail.SetErr(nil)
	remind(t, env, ritaDeadline.Add(-11*time.Hour))
	assert.Equal(t, []string{fixtures.UmbrellaOwnerRitaEmail}, recipients(env))
}

// A User who holds a Passkey but no Second factor is withheld after a password login once
// the grace ends (ADR 0051), so the reminder counts her grace like any other User's and
// names the Passkey as a way out.
func TestAPasskeyHolderWithoutASecondFactorIsRemindedAndToldBothWaysOut(t *testing.T) {
	env := testhelper.Setup(t)
	started := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.GlobexID).SetSecondFactorRequiredAt(started).Exec(t.Context()))

	remind(t, env, started.Add(6*24*time.Hour+time.Hour))

	pat := lo.Filter(env.SystemMail.Messages(), func(m messaging.EmailMessage, _ int) bool { return m.To == fixtures.PasskeyPatEmail })
	require.Len(t, pat, 1, "Pat holds a Passkey and no Second factor: her password sessions are withheld")
	assert.Contains(t, pat[0].Text, "Passkey")
	assert.Contains(t, pat[0].Text, "https://app.example/account/security")
}

package jobs_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestTelegramRedeliveryOldStopCannotUndoNewStart(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	first := env.PostTelegramUpdate(t, bot, said(1001, user, "/stop"))
	require.Equal(t, http.StatusOK, first.Code)
	second := env.PostTelegramUpdate(t, bot, said(1002, user, "/start"))
	require.Equal(t, http.StatusOK, second.Code)
	before := optedOut(t, env, user)
	require.False(t, before)
	retry := env.PostTelegramUpdate(t, bot, said(1001, user, "/stop"))
	require.Equal(t, http.StatusOK, retry.Code)
	after := optedOut(t, env, user)
	assert.False(t, after, "redelivering an old STOP must not undo the later START")
}

func TestTelegramRedeliveryOldStartCannotUndoNewStop(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	first := env.PostTelegramUpdate(t, bot, said(1001, user, "/start"))
	require.Equal(t, http.StatusOK, first.Code)
	second := env.PostTelegramUpdate(t, bot, said(1002, user, "/stop"))
	require.Equal(t, http.StatusOK, second.Code)
	before := optedOut(t, env, user)
	require.True(t, before)
	retry := env.PostTelegramUpdate(t, bot, said(1001, user, "/start"))
	require.Equal(t, http.StatusOK, retry.Code)
	after := optedOut(t, env, user)
	assert.True(t, after, "redelivering an old START must preserve the later STOP")
}

func TestTelegramRedeliveryOldUnblockCannotUndoNewBlock(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	first := env.PostTelegramUpdate(t, bot, memberStatus(1001, user, "member"))
	require.Equal(t, http.StatusOK, first.Code)
	second := env.PostTelegramUpdate(t, bot, memberStatus(1002, user, "kicked"))
	require.Equal(t, http.StatusOK, second.Code)
	before := optedOut(t, env, user)
	require.True(t, before)
	retry := env.PostTelegramUpdate(t, bot, memberStatus(1001, user, "member"))
	require.Equal(t, http.StatusOK, retry.Code)
	after := optedOut(t, env, user)
	assert.True(t, after, "redelivering an old unblock must preserve the later block")
}

func TestTelegramRedeliveryUnknownDestination(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeSecondTelegram(t)
	const unknownUser = 9191
	for _, input := range []string{said(1001, unknownUser, "/stop"), said(1002, unknownUser, "/start")} {
		reply := env.PostTelegramUpdate(t, bot, input)
		require.Equal(t, http.StatusOK, reply.Code)
	}
	before := optedOut(t, env, unknownUser)
	require.False(t, before)
	reply := env.PostTelegramUpdate(t, bot, said(1001, unknownUser, "/stop"))
	require.Equal(t, http.StatusOK, reply.Code)
	after := optedOut(t, env, unknownUser)
	assert.False(t, after, "an observation needs a receipt even without a Contact")
}

func TestTelegramRedeliveryOldButtonCannotUndoNewStart(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	press := pressed(1001, user, 77, "cb-audit", "unsubscribe")
	for _, input := range []string{press, said(1002, user, "/start")} {
		reply := env.PostTelegramUpdate(t, bot, input)
		require.Equal(t, http.StatusOK, reply.Code)
	}
	before := optedOut(t, env, user)
	require.False(t, before)
	reply := env.PostTelegramUpdate(t, bot, press)
	require.Equal(t, http.StatusOK, reply.Code)
	after := optedOut(t, env, user)
	assert.False(t, after, "an old unsubscribe button must not undo a later START")
}

func TestTelegramRedeliveryOldBlockCannotUndoNewUnblock(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	for _, input := range []string{memberStatus(1001, user, "kicked"), memberStatus(1002, user, "member")} {
		reply := env.PostTelegramUpdate(t, bot, input)
		require.Equal(t, http.StatusOK, reply.Code)
	}
	before := optedOut(t, env, user)
	require.False(t, before)
	reply := env.PostTelegramUpdate(t, bot, memberStatus(1001, user, "kicked"))
	require.Equal(t, http.StatusOK, reply.Code)
	after := optedOut(t, env, user)
	assert.False(t, after, "an old block must not undo a later unblock")
}

func TestTelegramRedeliveryStandingStopMustBeConsumed(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	user := mustInt(t, fixtures.BindingLiamDestination)
	for _, input := range []string{said(1001, user, "/stop"), said(1002, user, "/stop"), said(1003, user, "/start")} {
		reply := env.PostTelegramUpdate(t, bot, input)
		require.Equal(t, http.StatusOK, reply.Code)
	}
	before := optedOut(t, env, user)
	require.False(t, before)
	reply := env.PostTelegramUpdate(t, bot, said(1002, user, "/stop"))
	require.Equal(t, http.StatusOK, reply.Code)
	after := optedOut(t, env, user)
	assert.False(t, after, "a stop observed while already stopped still needs to be consumed")
}

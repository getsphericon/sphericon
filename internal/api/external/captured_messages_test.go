package external_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func globexClient(t *testing.T, env *testhelper.TestEnv, scopes ...string) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, scopes...))
}

func TestExternalCapturedMessagesAreListedAndReadWithTheReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	c := globexClient(t, env, "captured_messages:read")

	list, err := c.CapturedMessagesList(t.Context(), externalapi.CapturedMessagesListParams{})
	require.NoError(t, err)
	one, err := c.CapturedMessagesGet(t.Context(), externalapi.CapturedMessagesGetParams{ID: externalapi.EntityId(fixtures.CapturedMessageGlobexWelcomeID)})
	require.NoError(t, err)

	page, ok := list.(*externalapi.CapturedMessagesListOK)
	require.Truef(t, ok, "got %T", list)
	require.Len(t, page.Items, 2)
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexWelcomeID), int64(page.Items[0].ID), "newest first")
	msg, ok := one.(*externalapi.CapturedMessageResource)
	require.Truef(t, ok, "got %T", one)
	assert.Equal(t, fixtures.CapturedMessageGlobexWelcomeSubject, msg.Subject)
	assert.Contains(t, msg.HTML, "start here")
	assert.Equal(t, []string{"https://globex.test/start"}, msg.Links)
	assert.Len(t, msg.Headers, 3)
}

func TestExternalCapturedMessagesNeedTheirScopes(t *testing.T) {
	env := testhelper.Setup(t)
	reader := globexClient(t, env, "captured_messages:read")
	writer := globexClient(t, env, "captured_messages:write")
	id := externalapi.EntityId(fixtures.CapturedMessageGlobexWelcomeID)

	noRead, err := writer.CapturedMessagesList(t.Context(), externalapi.CapturedMessagesListParams{})
	require.NoError(t, err)
	noReadOne, err := writer.CapturedMessagesGet(t.Context(), externalapi.CapturedMessagesGetParams{ID: id})
	require.NoError(t, err)
	noWrite, err := reader.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: id})
	require.NoError(t, err)
	noWriteAll, err := reader.CapturedMessagesDeleteAll(t.Context())
	require.NoError(t, err)

	assert.IsType(t, &externalapi.CapturedMessagesListUnauthorized{}, noRead)
	assert.IsType(t, &externalapi.CapturedMessagesGetUnauthorized{}, noReadOne)
	assert.IsType(t, &externalapi.CapturedMessagesDeleteUnauthorized{}, noWrite)
	assert.IsType(t, &externalapi.ProblemDetails{}, noWriteAll)
	assert.Equal(t, 401, int(noWriteAll.(*externalapi.ProblemDetails).Status.Value))
}

func TestExternalCapturedMessagesAreDeletedOneAndAllWithinTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := globexClient(t, env, "captured_messages:read", "captured_messages:write")
	welcome := externalapi.EntityId(fixtures.CapturedMessageGlobexWelcomeID)

	one, err := c.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: welcome})
	require.NoError(t, err)
	again, err := c.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: welcome})
	require.NoError(t, err)
	foreign, err := c.CapturedMessagesGet(t.Context(), externalapi.CapturedMessagesGetParams{ID: externalapi.EntityId(fixtures.CapturedMessageUmbrellaID)})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.CapturedMessagesDeleteNoContent{}, one)
	assert.IsType(t, &externalapi.CapturedMessagesDeleteNotFound{}, again)
	assert.IsType(t, &externalapi.CapturedMessagesGetNotFound{}, foreign, "another Workspace's message is not found")

	all, err := c.CapturedMessagesDeleteAll(t.Context())
	require.NoError(t, err)
	assert.IsType(t, &externalapi.CapturedMessagesDeleteAllNoContent{}, all)
	list, err := c.CapturedMessagesList(t.Context(), externalapi.CapturedMessagesListParams{})
	require.NoError(t, err)
	assert.Empty(t, list.(*externalapi.CapturedMessagesListOK).Items)
	assert.Equal(t, 4, env.DB.CapturedMessage.Query().CountX(t.Context()), "the other Workspaces' messages are untouched")
}

func TestExternalCapturedMessageOfAnotherWorkspaceIsNeitherListedNorDeleted(t *testing.T) {
	env := testhelper.Setup(t)
	c := globexClient(t, env, "captured_messages:read", "captured_messages:write")
	foreign := externalapi.EntityId(fixtures.CapturedMessageUmbrellaID)

	list, err := c.CapturedMessagesList(t.Context(), externalapi.CapturedMessagesListParams{})
	require.NoError(t, err)
	deleted, err := c.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: foreign})
	require.NoError(t, err)

	for _, item := range list.(*externalapi.CapturedMessagesListOK).Items {
		assert.NotEqual(t, int64(foreign), int64(item.ID), "another Workspace's message is not listed")
	}
	assert.IsType(t, &externalapi.CapturedMessagesDeleteNotFound{}, deleted)
	assert.True(t, env.DB.CapturedMessage.Query().Where(capturedmessage.ID(int64(foreign))).ExistX(t.Context()), "and it survives")
}

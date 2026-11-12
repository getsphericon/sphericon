package external_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func globexClient(t *testing.T, env *testhelper.TestEnv, scopes ...apitokens.Scope) *externalapi.Client {
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

	page := list
	require.Len(t, page.Items, 2)
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexWelcomeID), int64(page.Items[0].ID), "newest first")
	msg := one
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

	_, noRead := writer.CapturedMessagesList(t.Context(), externalapi.CapturedMessagesListParams{})
	_, noReadOne := writer.CapturedMessagesGet(t.Context(), externalapi.CapturedMessagesGetParams{ID: id})
	noWrite := reader.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: id})
	noWriteAll := reader.CapturedMessagesDeleteAll(t.Context())

	testhelper.ExternalRefusal(t, noRead, http.StatusForbidden)
	testhelper.ExternalRefusal(t, noReadOne, http.StatusForbidden)
	testhelper.ExternalRefusal(t, noWrite, http.StatusForbidden)
	testhelper.ExternalRefusal(t, noWriteAll, http.StatusForbidden)
}

func TestExternalCapturedMessagesAreDeletedOneAndAllWithinTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := globexClient(t, env, "captured_messages:read", "captured_messages:write")
	welcome := externalapi.EntityId(fixtures.CapturedMessageGlobexWelcomeID)
	others := env.DB.CapturedMessage.Query().Where(capturedmessage.WorkspaceIDNEQ(fixtures.GlobexID)).CountX(t.Context())

	one := c.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: welcome})
	again := c.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: welcome})
	_, foreign := c.CapturedMessagesGet(t.Context(), externalapi.CapturedMessagesGetParams{ID: externalapi.EntityId(fixtures.CapturedMessageUmbrellaID)})

	require.NoError(t, one)
	testhelper.ExternalRefusal(t, again, http.StatusNotFound)
	testhelper.ExternalRefusal(t, foreign, http.StatusNotFound, "another Workspace's message is not found")

	err := c.CapturedMessagesDeleteAll(t.Context())
	require.NoError(t, err)
	list, err := c.CapturedMessagesList(t.Context(), externalapi.CapturedMessagesListParams{})
	require.NoError(t, err)
	assert.Empty(t, list.Items)
	assert.Equal(t, others, env.DB.CapturedMessage.Query().CountX(t.Context()), "the other Workspaces' messages are untouched")
}

func TestExternalCapturedMessageOfAnotherWorkspaceIsNeitherListedNorDeleted(t *testing.T) {
	env := testhelper.Setup(t)
	c := globexClient(t, env, "captured_messages:read", "captured_messages:write")
	foreign := externalapi.EntityId(fixtures.CapturedMessageUmbrellaID)

	list, err := c.CapturedMessagesList(t.Context(), externalapi.CapturedMessagesListParams{})
	require.NoError(t, err)
	deleted := c.CapturedMessagesDelete(t.Context(), externalapi.CapturedMessagesDeleteParams{ID: foreign})

	for _, item := range list.Items {
		assert.NotEqual(t, int64(foreign), int64(item.ID), "another Workspace's message is not listed")
	}
	testhelper.ExternalRefusal(t, deleted, http.StatusNotFound)
	assert.True(t, env.DB.CapturedMessage.Query().Where(capturedmessage.ID(int64(foreign))).ExistX(t.Context()), "and it survives")
}

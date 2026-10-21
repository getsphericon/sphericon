package server_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The secret of the archived Integration in fixtures/integrations.yml.
var secretArchived = newSecret("secret-C0123456789")

// An archived Integration (ADR 0049) still reads its provider's callbacks: the bounce and the
// complaint of mail already sent through it are facts Suppression must not lose. The hook is the
// only thing the Integration receives, and it carries no new data into the Workspace.
func TestHookOfAnArchivedIntegrationStillSuppressesABounce(t *testing.T) {
	env := setupHooks(t)

	rec := signedPost(t, env, "omik_test_integrationacmearchived", secretArchived, "msg_1", failureBody("evt_1", "late@example.com", true))

	require.Equal(t, http.StatusOK, rec.Code)
	applySuppression(t, env)
	assert.True(t, suppressed(t, env, "late@example.com"))
}

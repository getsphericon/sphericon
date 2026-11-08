package auditlog_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Instance without the Enterprise Edition's Audit log refuses every read of it, and its
// retention window, as unlicensed: the refusal every surface answers alike.
func TestTheAuditLogOfAnInstanceWithoutItsLicenseIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	log := auditlog.New(nil, workspaces.New(env.DB, env.Bus, time.Now))

	_, err := log.Page(t.Context(), s, auditlog.Query{Limit: 10})
	require.ErrorIs(t, err, auditlog.ErrUnlicensed)
	_, err = log.Facets(t.Context(), s)
	require.ErrorIs(t, err, auditlog.ErrUnlicensed)
	_, err = log.Export(t.Context(), s, events.AuditFilter{})
	require.ErrorIs(t, err, auditlog.ErrUnlicensed)
	_, err = log.Retention(t.Context(), s)
	require.ErrorIs(t, err, auditlog.ErrRetentionUnlicensed)
	err = log.SetRetention(t.Context(), s, new(30))
	require.ErrorIs(t, err, auditlog.ErrRetentionUnlicensed)
	assert.Nil(t, env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).RetentionDays, "the window stays unset")
}

// A cursor is an entry id the log handed out: anything else names nothing.
func TestACursorThatIsNoEntryIdIsAnUnknownReference(t *testing.T) {
	_, err := auditlog.Cursor("abc")
	require.ErrorIs(t, err, auditlog.ErrBadCursor)
	_, err = auditlog.Cursor("0")
	require.ErrorIs(t, err, auditlog.ErrBadCursor)
	_, err = auditlog.Cursor("-4")
	require.ErrorIs(t, err, auditlog.ErrBadCursor)
	first, err := auditlog.Cursor("")
	require.NoError(t, err)
	assert.Zero(t, first, "no cursor starts from the newest entry")
}

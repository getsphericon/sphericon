package testhelper

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// pgtestdb serializes a template's creation with an advisory lock, which only excludes
// sessions connected to the same database: RealDB must always ask through the shared test
// database, even once Setup has pointed DATABASE_URL at the process's own leased one.
func TestRealDBTemplateLockIsTakenOnTheSharedDatabaseAfterSetup(t *testing.T) {
	before := serverConfig(t).Database
	initBaseline()
	require.NoError(t, errLoad)

	after := serverConfig(t).Database
	require.Equal(t, before, after)
	own, err := url.Parse(baseCfg.DatabaseURL)
	require.NoError(t, err)
	require.NotEqual(t, after, own.Path[1:], "the process has a database of its own")
}

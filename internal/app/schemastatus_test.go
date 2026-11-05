package app_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

// `migrate --status --json` is what `upgrade` reads from the release it is about to install: the
// database's state and the binary's table of Majors.
func TestWriteStatusJSONCarriesTheStateAndTheTableOfMajors(t *testing.T) {
	url := testhelper.EmptyDBURL(t)

	var out bytes.Buffer
	require.NoError(t, app.WriteSchemaBoundaries(t.Context(), url, &out))

	var b upgrade.Boundaries
	require.NoError(t, json.Unmarshal(out.Bytes(), &b))
	require.Equal(t, 1, b.State.Major, "an empty database reports Major 1 until it is migrated")
	require.Len(t, b.Majors, len(db.Majors))
	require.Equal(t, db.Majors[len(db.Majors)-1].Major, b.Majors[len(b.Majors)-1].Major)
}

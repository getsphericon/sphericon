package fixtures_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var update = flag.Bool("update", false, "rewrite the body of every fixture row that holds a document from its render")

// Fixtures load without the save path, so a row with a document carries a committed body. Every
// such body must be what the real renderer makes of the row's document (ADR 0040): a dependency
// update that changes the export fails here, and -update rewrites the rows.
func TestFixtureTemplateBodiesAreTheRenderOfTheirDocuments(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	rows := env.DB.EmailTemplate.Query().Where(emailtemplate.DocNotNil()).AllX(ctx)
	require.NotEmpty(t, rows, "a fixture Template is written in the editor")

	rewrites := map[int64]string{}
	for _, row := range rows {
		got, err := emailrender.FromDocument(ctx, &row.Subject, row.Doc)
		require.NoError(t, err, "template %d", row.ID)
		require.NotNil(t, row.EditorVersion, "template %d", row.ID)
		require.Equal(t, emaildoc.EditorVersion(), *row.EditorVersion, "template %d: the fixture names the editor version of the bundle", row.ID)
		if row.Body != got.Body {
			rewrites[row.ID] = got.Body
		}
	}

	if *update && len(rewrites) > 0 {
		rewriteFixtureBodies(t, rewrites)
		return
	}
	require.Empty(t, keys(rewrites), "these fixture Templates have a body that is not the render of their document (rerun with -update)")
}

func keys(m map[int64]string) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// rewriteFixtureBodies replaces the `body:` line of each named row in fixtures/email_templates.yml
// with the new body, as a double-quoted YAML scalar (a JSON string is one).
func rewriteFixtureBodies(t *testing.T, bodies map[int64]string) {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", "email_templates.yml")
	src, err := os.ReadFile(path)
	require.NoError(t, err)

	for id, body := range bodies {
		var quoted bytes.Buffer
		enc := json.NewEncoder(&quoted)
		enc.SetEscapeHTML(false)
		require.NoError(t, enc.Encode(body))

		row := regexp.MustCompile(`(?s)(- id: ` + strconv.FormatInt(id, 10) + `\b.*?\n  body: )[^\n]*`)
		require.True(t, row.Match(src), "template %d has no body line in the fixture file", id)
		src = row.ReplaceAll(src, append([]byte("${1}"), bytes.TrimRight(quoted.Bytes(), "\n")...))
	}
	require.NoError(t, os.WriteFile(path, src, 0o644))
}

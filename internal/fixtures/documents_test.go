package fixtures_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
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

	messages := make([]authored, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, authored{id: row.ID, subject: row.Subject, doc: row.Doc, version: row.EditorVersion, body: row.Body})
	}
	requireBodiesAreRenders(t, "email_templates.yml", messages)
}

func TestFixtureBroadcastBodiesAreTheRenderOfTheirDocuments(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	rows := env.DB.Broadcast.Query().Where(broadcast.DocNotNil()).AllX(ctx)
	require.NotEmpty(t, rows, "a fixture Broadcast is written in the editor")

	messages := make([]authored, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, authored{id: row.ID, subject: row.Subject, doc: row.Doc, version: row.EditorVersion, body: row.Body})
	}
	requireBodiesAreRenders(t, "broadcasts.yml", messages)
}

// authored is a fixture row that holds a document, whatever table it sits in.
type authored struct {
	id      int64
	subject string
	doc     json.RawMessage
	version *string
	body    string
}

func requireBodiesAreRenders(t *testing.T, file string, messages []authored) {
	t.Helper()
	ctx := t.Context()
	rewrites := map[int64]string{}
	for _, m := range messages {
		got, err := emailrender.FromDocument(ctx, testhelper.RealRenderer(), &m.subject, m.doc)
		require.NoError(t, err, "%s row %d", file, m.id)
		require.NotNil(t, m.version, "%s row %d", file, m.id)
		require.Equal(t, emaildoc.EditorVersion(), *m.version, "%s row %d: the fixture names the editor version of the bundle", file, m.id)
		if m.body != got.Body {
			rewrites[m.id] = got.Body
		}
	}

	if *update && len(rewrites) > 0 {
		rewriteFixtureBodies(t, file, rewrites)
		return
	}
	require.Empty(t, slices.Sorted(maps.Keys(rewrites)), "these rows of %s have a body that is not the render of their document (rerun with -update)", file)
}

// rewriteFixtureBodies replaces the `body:` line of each named row in the fixture file
// with the new body, as a double-quoted YAML scalar (a JSON string is one).
func rewriteFixtureBodies(t *testing.T, file string, bodies map[int64]string) {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", file)
	src, err := os.ReadFile(path)
	require.NoError(t, err)

	for id, body := range bodies {
		var quoted bytes.Buffer
		enc := json.NewEncoder(&quoted)
		enc.SetEscapeHTML(false)
		require.NoError(t, enc.Encode(body))

		row := regexp.MustCompile(`(?s)(- id: ` + strconv.FormatInt(id, 10) + `\b.*?\n  body: )[^\n]*`)
		require.True(t, row.Match(src), "row %d has no body line in %s", id, file)
		src = row.ReplaceAll(src, append([]byte("${1}"), bytes.TrimRight(quoted.Bytes(), "\n")...))
	}
	require.NoError(t, os.WriteFile(path, src, 0o644))
}

package fixturegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures/fixturegen"
)

// A fixture file may repeat a row with a template loop. The generator reads the raw file, so it
// names the rows of a loop body once and still reads the annotated rows around it.
func TestGenerateReadsAFileWithATemplateLoop(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"events.yml": "- id: 1 # fixture: First\n  email: first@example.com\n" +
			"# {{range $i := seq 100 199}}\n- id: '{{$i}}'\n  email: 'bulk-{{$i}}@example.com'\n# {{end}}\n" +
			"- id: 2 # fixture: Last\n  email: last@example.com\n",
	})

	src, err := fixturegen.Generate(dir)

	require.NoError(t, err)
	out := string(src)
	require.Contains(t, out, "FirstID")
	require.Contains(t, out, "LastID")
	require.NotContains(t, out, "bulk-")
}

// A catalogued row repeated by a loop would name many rows with one constant.
func TestGenerateRefusesAnAnnotationInsideATemplateLoop(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"events.yml": "{{range $i := seq 1 3}}\n- id: '{{$i}}' # fixture: Repeated\n  email: a@example.com\n{{end}}\n",
	})

	_, err := fixturegen.Generate(dir)

	require.ErrorContains(t, err, "Repeated")
}

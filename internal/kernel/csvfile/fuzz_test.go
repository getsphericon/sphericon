package csvfile_test

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/csvfile"
)

// Whatever text a cell holds, the writer writes a file that reads back with the BOM first and no
// cell a spreadsheet would run.
func FuzzNoWrittenCellStartsAFormula(f *testing.F) {
	for _, cell := range []string{"=1+1", "+1", "-5", "@SUM(A1)", "\tx", "\rx", "\r\n=x", "", " =x", `"=x"`, "a,b", "x\n=y", "'", "\xff"} {
		f.Add(cell, "plain", cell)
	}

	f.Fuzz(func(t *testing.T, a, b, c string) {
		var buf bytes.Buffer
		w := csvfile.NewWriter(&buf)

		require.NoError(t, w.Write([]string{a, b, c}))
		require.NoError(t, w.Flush())

		body, found := strings.CutPrefix(buf.String(), bom)
		assert.True(t, found, "the file begins with a BOM")
		assert.Falsef(t, strings.ContainsAny(firstCharacters(t, body), "=+-@\t\r"), "%q", body)
	})
}

// firstCharacters reads a CSV file and returns the first character of each of its non-empty cells.
func firstCharacters(t *testing.T, body string) string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	require.NoError(t, err)
	var firsts strings.Builder
	for _, record := range records {
		for _, cell := range record {
			if cell != "" {
				firsts.WriteByte(cell[0])
			}
		}
	}
	return firsts.String()
}

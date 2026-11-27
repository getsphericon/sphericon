package csvfile_test

import (
	"bytes"
	"encoding/csv"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/kernel/csvfile"
)

const bom = "\xEF\xBB\xBF"

func TestACellASpreadsheetWouldRunIsWrittenWithALeadingApostrophe(t *testing.T) {
	cases := []struct {
		name string
		cell string
		want string
	}{
		{"equals", `=HYPERLINK("http://evil")`, `"'=HYPERLINK(""http://evil"")"`},
		{"plus", "+1", "'+1"},
		{"minus", "-5", "'-5"},
		{"at", "@SUM(A1)", "'@SUM(A1)"},
		{"tab", "\tx", "'\tx"},
		{"carriage return", "\rx", "\"'\rx\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := csvfile.NewWriter(&buf)

			require.NoError(t, w.Write([]string{c.cell}))
			require.NoError(t, w.Flush())

			assert.Equal(t, bom+c.want+"\n", buf.String())
		})
	}
}

func TestAnyOtherCellIsWrittenAsGivenQuotedAsCSVRequires(t *testing.T) {
	var buf bytes.Buffer
	w := csvfile.NewWriter(&buf)
	record := []string{"Ada", "a,b", `say "hi"`, "two\nlines", "", " lead", "x=1", "'=quoted"}

	require.NoError(t, w.Write(record))
	require.NoError(t, w.Flush())

	assert.Equal(t, bom+"Ada,\"a,b\",\"say \"\"hi\"\"\",\"two\nlines\",,\" lead\",x=1,'=quoted\n", buf.String())
}

func TestTheFileBeginsWithOneBOMHoweverOftenItIsFlushed(t *testing.T) {
	var buf bytes.Buffer
	w := csvfile.NewWriter(&buf)
	record := []string{"=1", "b"}

	require.NoError(t, w.Write(record))
	require.NoError(t, w.Flush())
	require.NoError(t, w.Write([]string{"c", "d"}))
	require.NoError(t, w.Flush())

	assert.Equal(t, bom+"'=1,b\nc,d\n", buf.String())
	assert.Equal(t, []string{"=1", "b"}, record, "the caller's record is left as it was")
}

func TestAWriterThatWritesNoRecordWritesNothing(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, csvfile.NewWriter(&buf).Flush())

	assert.Empty(t, buf.String())
}

// pages yields each page of records, then ends with end; stopped says whether its reader stopped
// it before the end.
type pages struct {
	items   [][][]string
	end     error
	stopped bool
}

func (p *pages) seq() iter.Seq2[[][]string, error] {
	return func(yield func([][]string, error) bool) {
		for _, page := range p.items {
			if !yield(page, nil) {
				p.stopped = true
				return
			}
		}
		if p.end != nil {
			yield(nil, p.end)
		}
	}
}

func TestEachPageOfRecordsIsAChunkOfTheFile(t *testing.T) {
	p := &pages{items: [][][]string{{{"name", "note"}}, {{"Ada", "=1"}, {"Bob", "a,b"}}, {}}}

	var got []string
	for chunk, err := range csvfile.Chunks(p.seq()) {
		require.NoError(t, err)
		got = append(got, string(chunk))
	}

	assert.Equal(t, []string{bom + "name,note\n", "Ada,'=1\nBob,\"a,b\"\n", ""}, got)
}

func TestTheChunksEndWithTheErrorThePagesEndWith(t *testing.T) {
	failed := errors.New("read failed")
	p := &pages{items: [][][]string{{{"name"}}}, end: failed}

	var got []error
	for _, err := range csvfile.Chunks(p.seq()) {
		got = append(got, err)
	}

	assert.Equal(t, []error{nil, failed}, got)
}

func TestAReaderThatStopsStopsThePages(t *testing.T) {
	p := &pages{items: [][][]string{{{"name"}}, {{"Ada"}}, {{"Bob"}}}}

	for _, err := range csvfile.Chunks(p.seq()) {
		require.NoError(t, err)
		break
	}

	assert.True(t, p.stopped)
}

// A list held in one cell (a Contact's Tags) is one CSV record, which a CSV reader gives back as the
// list whatever its names hold: a comma, a quote or a line feed. A carriage return is left out: a
// CSV reader folds a quoted CRLF into a line feed, whoever wrote it.
func TestAListInOneCellReadsBackAsTheList(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		names := rapid.SliceOfN(rapid.StringMatching(`[^\x00\r]{1,12}`), 1, 6).Draw(t, "names")

		cell := csvfile.List(names)

		read, err := csv.NewReader(strings.NewReader(cell)).Read()
		require.NoError(t, err)
		assert.Equal(t, names, read)
	})
}

func TestAListOfPlainNamesIsTheNamesJoinedByCommas(t *testing.T) {
	assert.Equal(t, "VIP,from Mailchimp", csvfile.List([]string{"VIP", "from Mailchimp"}))
	assert.Equal(t, `"a, b",c`, csvfile.List([]string{"a, b", "c"}))
	assert.Empty(t, csvfile.List(nil))
}

// Package csvfile is the one way the app writes CSV (ADR 0138): a file a spreadsheet opens
// without running any of its cells. It begins with a UTF-8 BOM, is comma-delimited, and a cell
// beginning with =, +, -, @, a tab or a carriage return is written with a leading apostrophe, since
// a spreadsheet evaluates such a cell and most cells the app writes are untrusted text (ADR 0132).
// No other package constructs the standard library's CSV writer (ast-grep rule
// one-csv-writer).
package csvfile

import (
	"bytes"
	"encoding/csv"
	"io"
	"iter"
	"strings"
)

// bom marks the file as UTF-8 for a spreadsheet that would otherwise guess its encoding.
const bom = "\xEF\xBB\xBF"

// formulaStarts are the first characters a spreadsheet reads as the start of a formula.
const formulaStarts = "=+-@\t\r"

// Writer writes records to an io.Writer as a CSV file, buffered until Flush.
type Writer struct {
	dst     io.Writer
	csv     *csv.Writer
	started bool
}

// NewWriter returns a Writer to w. Nothing is written to w before the first record.
func NewWriter(w io.Writer) *Writer {
	return &Writer{dst: w, csv: csv.NewWriter(w)}
}

// Write writes one record, the BOM first if it is the file's first; the caller's record is left
// as it is. The record may sit in the buffer until Flush.
func (w *Writer) Write(record []string) error {
	if !w.started {
		if _, err := io.WriteString(w.dst, bom); err != nil {
			return err
		}
		w.started = true
	}
	cells := make([]string, len(record))
	for i, cell := range record {
		cells[i] = neutralise(cell)
	}
	return w.csv.Write(cells)
}

// Flush writes the buffered records and returns the error of any write so far.
func (w *Writer) Flush() error {
	w.csv.Flush()
	return w.csv.Error()
}

// neutralise prefixes a cell a spreadsheet would run with an apostrophe.
func neutralise(cell string) string {
	if cell != "" && strings.IndexByte(formulaStarts, cell[0]) >= 0 {
		return "'" + cell
	}
	return cell
}

// Chunks writes each page of records as one chunk of a CSV file, the BOM in the first, for a
// module that yields a file instead of writing it (ADR 0111) and a driver that streams it
// (internal/kernel/seqreader). A chunk is the caller's to keep. The pages are read only as the
// chunks are; an error of the pages, or of the writer, ends the chunks with it.
func Chunks(pages iter.Seq2[[][]string, error]) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		var buf bytes.Buffer
		w := NewWriter(&buf)
		for page, err := range pages {
			if err != nil {
				yield(nil, err)
				return
			}
			if err := w.writePage(page); err != nil {
				yield(nil, err)
				return
			}
			if !yield(bytes.Clone(buf.Bytes()), nil) {
				return
			}
			buf.Reset()
		}
	}
}

// List is a list of values held in one cell (a Contact's Tags, the scopes of its opt-outs): one
// CSV record, quoted as CSV requires, with no line end. A CSV reader gives the list back; the cell
// itself is neutralised as any other when a Writer writes it. No values is "".
func List(values []string) string {
	if len(values) == 0 {
		return ""
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	// A strings.Builder takes every write, so neither Write nor Flush can fail here.
	if err := w.Write(values); err != nil {
		panic("csvfile: a record written to memory failed: " + err.Error())
	}
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n")
}

// writePage writes the records of a page and flushes them.
func (w *Writer) writePage(page [][]string) error {
	for _, record := range page {
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return w.Flush()
}

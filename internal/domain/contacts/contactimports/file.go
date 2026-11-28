package contactimports

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"iter"
	"unicode/utf8"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/contactimportchunk"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ChunkSize is the size of the parts a file is kept in (ADR 0138): its job reads from the chunk its
// checkpoint names rather than the whole file. Every chunk but the last is this size.
const ChunkSize = 1 << 20

// bom is the UTF-8 byte order mark a spreadsheet may begin its file with.
var bom = []byte("\xEF\xBB\xBF")

// Refusals of an upload: the file is refused whole.
var (
	ErrNotUTF8 = validation.Refuse("contactimports: the file is not UTF-8", i18n.ErrorsContactImportNotUtf8)
	// ErrNoHeader means the file has no row, so no header.
	ErrNoHeader = validation.Refuse("contactimports: the file has no header row", i18n.ErrorsContactImportNoHeader)
	// ErrTooManyColumns means the header has more columns than an import maps.
	ErrTooManyColumns = validation.Refuse("contactimports: the file has too many columns", i18n.ErrorsContactImportTooManyColumns)
	// ErrUnparsable means the file cannot be read as CSV: the refusal names the line.
	ErrUnparsable = validation.Refuse("contactimports: the file cannot be read as CSV", i18n.ErrorsContactImportUnparsable)
)

// UploadInput is an uploaded file and the Membership of the Member who uploads it.
type UploadInput struct {
	File         io.Reader
	MembershipID int64
}

// Upload keeps a CSV file of Contacts as a new import of the Workspace, `uploaded`, after reading
// it whole: it must be UTF-8 (a byte order mark is allowed and dropped), its cells separated by
// commas, semicolons or tabs (whichever its header row has most of outside quotes), its first row a
// header of at most the contract's columns, and every row readable by the standard library's CSV
// reader; anything else is refused whole. The rows below the header are counted, and those with
// another number of cells than the header, which the run refuses. The file is kept as chunks of
// ChunkSize in the import's transaction.
func (m *Module) Upload(ctx context.Context, s *ent.Scoped, in UploadInput) (View, error) {
	data, err := io.ReadAll(in.File)
	if err != nil {
		return View{}, err
	}
	facts, err := Inspect(data)
	if err != nil {
		return View{}, err
	}
	data = facts.Body
	ms, err := s.Membership().Query().Where(membership.ID(in.MembershipID)).WithUser().Only(ctx)
	if err != nil {
		return View{}, err
	}
	var imp *ent.ContactImport
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		imp, err = ts.ContactImport().Create().
			SetMembershipID(in.MembershipID).
			SetMemberName(ms.Edges.User.Name).
			SetDelimiter(facts.Delimiter).
			SetHeader(facts.Header).
			SetSize(int64(len(data))).
			SetRowCount(facts.Rows).
			SetMalformedRows(facts.Malformed).
			Save(ctx)
		if err != nil {
			return err
		}
		chunks := make([]*ent.ContactImportChunkScopedCreate, 0, len(data)/ChunkSize+1)
		for seq := 0; seq*ChunkSize < len(data); seq++ {
			part := data[seq*ChunkSize : min((seq+1)*ChunkSize, len(data))]
			chunks = append(chunks, ts.ContactImportChunk().Create().SetContactImportID(imp.ID).SetSeq(seq).SetData(part))
		}
		return ts.ContactImportChunk().CreateBulk(chunks...).Exec(ctx)
	})
	if err != nil {
		return View{}, err
	}
	return m.view(ctx, s, imp)
}

// File is what an upload reads of its file: its body without the byte order mark, the delimiter
// of its cells, its header, its rows below the header and those with another number of cells.
type File struct {
	Body      []byte
	Delimiter contactimport.Delimiter
	Header    []string
	Rows      int
	Malformed int
}

// Inspect reads a file whole as Upload keeps it, or refuses it as Upload says: ErrNotUTF8,
// ErrNoHeader, ErrTooManyColumns or ErrUnparsable.
func Inspect(data []byte) (File, error) {
	data = bytes.TrimPrefix(data, bom)
	if !utf8.Valid(data) {
		return File{}, ErrNotUTF8
	}
	f := File{Body: data, Delimiter: detectDelimiter(data)}
	r := newReader(bytes.NewReader(data), f.Delimiter)
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return File{}, ErrNoHeader
	}
	if err != nil {
		return File{}, unparsable(err)
	}
	if len(header) > bounds.ContactImportColumnsMaxItems {
		return File{}, ErrTooManyColumns.Args(map[string]any{"Max": bounds.ContactImportColumnsMaxItems})
	}
	f.Header = header
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			return f, nil
		}
		if err != nil {
			return File{}, unparsable(err)
		}
		f.Rows++
		if len(record) != len(header) {
			f.Malformed++
		}
	}
}

// unparsable is ErrUnparsable naming the line the reader stopped at.
func unparsable(err error) error {
	line := 0
	if pe, ok := errors.AsType[*csv.ParseError](err); ok {
		line = pe.Line
	}
	return ErrUnparsable.Args(map[string]any{"Line": line})
}

// delimiters are the characters a file may separate its cells by, by the ent enum.
var delimiters = map[contactimport.Delimiter]rune{
	contactimport.DelimiterComma:     ',',
	contactimport.DelimiterSemicolon: ';',
	contactimport.DelimiterTab:       '\t',
}

// detectDelimiter is the delimiter a file's header row has most of outside quotes; a comma when it
// has none.
func detectDelimiter(data []byte) contactimport.Delimiter {
	counts := map[rune]int{}
	quoted := false
	for _, c := range string(data) {
		if c == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && (c == '\n' || c == '\r') {
			break
		}
		if !quoted {
			counts[c]++
		}
	}
	best := contactimport.DelimiterComma
	for _, d := range []contactimport.Delimiter{contactimport.DelimiterSemicolon, contactimport.DelimiterTab} {
		if counts[delimiters[d]] > counts[delimiters[best]] {
			best = d
		}
	}
	return best
}

// newReader is the standard library's CSV reader of a file with the delimiter: any number of cells
// per row (a row whose count differs is refused by the run, not the reader), quotes strict.
func newReader(r io.Reader, d contactimport.Delimiter) *csv.Reader {
	reader := csv.NewReader(r)
	reader.Comma = delimiters[d]
	reader.FieldsPerRecord = -1
	return reader
}

// fileOf reads the file of imp from its chunks, in order, one chunk at a time, from the chunk seq
// at offset: the reader of a run that resumes, or of the whole file from 0, 0.
func fileOf(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport, seq int, offset int64) io.Reader {
	load := func(seq int) ([]byte, bool, error) {
		chunk, err := s.ContactImportChunk().Query().
			Where(contactimportchunk.ContactImportID(imp.ID), contactimportchunk.Seq(seq)).
			Only(ctx)
		if ent.IsNotFound(err) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("read chunk %d: %w", seq, err)
		}
		return chunk.Data, true, nil
	}
	return &chunkReader{load: load, seq: seq, offset: offset}
}

// chunkReader reads a file's chunks in order, loading each (load: its bytes, false past the last)
// when the one before it is read.
type chunkReader struct {
	load    func(seq int) ([]byte, bool, error)
	seq     int
	offset  int64
	current []byte
	done    bool
}

func (c *chunkReader) Read(p []byte) (int, error) {
	for len(c.current) == 0 {
		if c.done {
			return 0, io.EOF
		}
		data, ok, err := c.load(c.seq)
		if err != nil {
			return 0, err
		}
		if !ok {
			c.done = true
			continue
		}
		c.current = data[min(c.offset, int64(len(data))):]
		c.seq++
		c.offset = 0
	}
	n := copy(p, c.current)
	c.current = c.current[n:]
	return n, nil
}

// record is a row of the file below its header: its number, from 1, its cells, and the offset in
// the file where the row after it begins.
type record struct {
	row   int
	cells []string
	next  int64
}

// records are the rows of imp's file below its header, from the start.
func records(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport) iter.Seq2[record, error] {
	return recordsFrom(ctx, s, imp, 0, 0)
}

// recordsFrom are the rows of imp's file from the offset in it where the row after row begins: the
// header is skipped when row is 0, so the first is row 1.
func recordsFrom(ctx context.Context, s *ent.Scoped, imp *ent.ContactImport, row int, offset int64) iter.Seq2[record, error] {
	return func(yield func(record, error) bool) {
		seq := int(offset / ChunkSize)
		r := newReader(fileOf(ctx, s, imp, seq, offset%ChunkSize), imp.Delimiter)
		if row == 0 {
			if _, err := r.Read(); err != nil {
				yield(record{}, err)
				return
			}
		}
		for {
			cells, err := r.Read()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(record{}, err)
				return
			}
			row++
			if !yield(record{row: row, cells: cells, next: offset + r.InputOffset()}, nil) {
				return
			}
		}
	}
}

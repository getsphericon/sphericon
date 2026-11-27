package contactimports

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/tagname"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Preview is what an import would do (ADR 0138): its first rows as it would write them, and counts
// over the whole file.
type Preview struct {
	Rows []PreviewRow
	// TotalRows are the rows below the header; RowsWithoutKey those that name no alias key and
	// MalformedRows those with another number of cells than the header, both refused.
	TotalRows, RowsWithoutKey, MalformedRows int
	// DistinctTags are the Tags the rows it would write add, the Tag for every row included.
	DistinctTags int
}

// PreviewRow is a row of the file as the import would write it: its cells as written, why it
// would be refused (nil when it would be written), the values it would drop and the traits of its
// address worth a look (ADR 0117).
type PreviewRow struct {
	Row      int
	Cells    []string
	Refusal  *csvimport.Issue
	Dropped  []csvimport.Issue
	Warnings []contacts.Warning
}

// Preview previews the import id with its mapping (the saved one, else the proposal), writing
// nothing: the first rows, each read as the run reads it and, when it reaches the Contact intake,
// judged by it as it would be (contacts.Module.Verdicts: the address check of ADR 0117 among
// them), and over the whole file the rows, the rows without a key, the rows of another cell count
// and the distinct Tags. Whether a row is new to the Workspace is not counted: it needs a lookup per
// row.
func (m *Module) Preview(ctx context.Context, s *ent.Scoped, id int64) (Preview, error) {
	v, err := m.Get(ctx, s, id)
	if err != nil {
		return Preview{}, err
	}
	rd, err := readingOf(ctx, s, v.Import.Header, v.Mapping)
	if err != nil {
		return Preview{}, err
	}
	var (
		out  Preview
		read []readRow
		tags = map[string]bool{}
	)
	for rec, err := range records(ctx, s, v.Import) {
		if err != nil {
			return Preview{}, err
		}
		rr := rd.row(rec.cells)
		out.TotalRows++
		if len(read) < sampleRows {
			read = append(read, rr)
			out.Rows = append(out.Rows, PreviewRow{Row: rec.row, Cells: rec.cells, Refusal: rr.refusal, Dropped: rr.dropped})
		}
		switch {
		case rr.refusal == nil:
			for _, name := range rr.in.Tags {
				tags[tagname.Key(name)] = true
			}
		case rr.refusal.Reason == csvimport.ReasonNoKey:
			out.RowsWithoutKey++
		default:
			out.MalformedRows++
		}
	}
	out.DistinctTags = len(tags)
	return out, m.judge(ctx, s, out.Rows, read)
}

// judge adds to each previewed row the intake would see its verdict: its refusal, or the traits of
// its address.
func (m *Module) judge(ctx context.Context, s *ent.Scoped, rows []PreviewRow, read []readRow) error {
	at := lo.FilterMap(read, func(rr readRow, i int) (int, bool) { return i, rr.refusal == nil })
	if len(at) == 0 {
		return nil
	}
	verdicts, err := m.contacts.Verdicts(ctx, s, lo.Map(at, func(i int, _ int) contacts.IntakeInput { return read[i].in }))
	if err != nil {
		return err
	}
	for j, i := range at {
		rows[i].Warnings = verdicts[j].Warnings
		if verdicts[j].Refusal == nil {
			continue
		}
		shown, _ := validation.Shown(verdicts[j].Refusal)
		refusal := refusedIssue(shown)
		rows[i].Refusal = &refusal
	}
	return nil
}

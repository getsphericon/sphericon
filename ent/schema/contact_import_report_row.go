package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
)

// ContactImportReportRow is a row of a CSV import's file that was not written whole (ADR 0138): a
// row it refused, or one it wrote without some of its values. The refused rows' report is written
// from them when it is downloaded; they are deleted after a download that completed, after seven
// days, and with their import. Personal data: they hold the file's cells, and no Erasure searches
// them (ADR 0138), their expiry bounds them. Only internal/domain/contacts/contactimports writes it.
type ContactImportReportRow struct {
	ent.Schema
}

func (ContactImportReportRow) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("contact_import_report_rows", ContactImportReportRow{}.Fields())
	a.Checks["contact_import_report_rows_cells_check"] = jsonCheck("cells", "array")
	a.Checks["contact_import_report_rows_issues_check"] = jsonCheck("issues", "array")
	a.Checks["contact_import_report_rows_row_check"] = "row >= 1"
	return []schema.Annotation{a}
}

func (ContactImportReportRow) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "contact_import_report_rows"}}
}

func (ContactImportReportRow) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("contact_import_id").
			Immutable(),
		// The row's number below the header, from 1.
		field.Int("row").
			Immutable(),
		// The row's cells as the file writes them.
		field.JSON("cells", []string{}).
			Immutable(),
		// Why the row was refused, or the values it was written without: an issue of the whole
		// row (no column) refuses it.
		field.JSON("issues", []csvimport.Issue{}).
			Immutable(),
	}
}

func (ContactImportReportRow) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("contact_import", ContactImport.Type).
			Ref("report_rows").
			Field("contact_import_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (ContactImportReportRow) Indexes() []ent.Index {
	return []ent.Index{
		// A report's rows in the file's order; a batch run again writes a row once.
		index.Fields("workspace_id", "contact_import_id", "row").
			Unique(),
	}
}

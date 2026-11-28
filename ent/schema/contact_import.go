package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
)

// ContactImport is a CSV import of Contacts (ADR 0138), one row per uploaded file: the Member who
// uploaded and runs it, its state, the facts of its file read at upload, its mapping and choices,
// where its run stands and what it wrote. The file itself is its ContactImportChunks and the rows
// it did not write whole its ContactImportReportRows. Only internal/domain/contacts/contactimports
// writes it.
type ContactImport struct {
	ent.Schema
}

// Annotations adds to the enum checks that the counts are never negative and the header a list.
func (ContactImport) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("contact_imports", ContactImport{}.Fields())
	a.Checks["contact_imports_header_check"] = jsonCheck("header", "array")
	a.Checks["contact_imports_recoveries_check"] = "recoveries >= 0"
	a.Checks["contact_imports_counts_check"] = "row_count >= 0 AND malformed_rows >= 0 AND processed >= 0 AND created >= 0 AND updated >= 0 AND refused >= 0 AND dropped >= 0 AND opted_out >= 0 AND suppressed >= 0"
	return []schema.Annotation{a}
}

func (ContactImport) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "contact_imports"}}
}

func (ContactImport) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The Membership of the Member who uploaded the file and runs it: the job acts as its
		// User. Null once the Member left the Workspace (SET NULL), which stops a run.
		field.Int64("membership_id").
			Optional().
			Nillable(),
		// uploaded until a mapping is saved, ready to run, running while its job writes the rows,
		// then done; cancelled and failed end it early. Moved only by the module's machine.
		field.Enum("state").
			Annotations(Machine()).
			Values("uploaded", "ready", "running", "cancelled", "failed", "done").
			Default("uploaded"),
		// The facts of the file, read once at upload: the character its cells are separated by,
		// its header, its size in bytes, its rows below the header and those whose cell count
		// differs from the header's.
		field.Enum("delimiter").
			Values("comma", "semicolon", "tab").
			Immutable(),
		field.JSON("header", []string{}).
			Immutable(),
		field.Int64("size").
			Immutable(),
		field.Int("row_count").
			Immutable(),
		field.Int("malformed_rows").
			Immutable(),
		// How the file is read: a column per column of the file and the choices for every row.
		// Null until the Member saves one; written again until the import runs.
		field.JSON("mapping", csvimport.Mapping{}).
			Optional(),
		// "Only add new Contacts": a Contact the Workspace has is left as it is but for the
		// refusals its row carries.
		field.Bool("only_add_new").
			Annotations(Attribute(reasonSetting)).
			Default(false),
		// "Treat them as new sign-ups": the import's Events name it but are not history.
		field.Bool("as_new_sign_ups").
			Annotations(Attribute(reasonSetting)).
			Default(false),
		// Where the run stands after its last batch: the next row to read below the header (0
		// before the first), and the chunk and the offset in it where that row begins.
		field.Int("checkpoint_row").
			Default(0),
		field.Int("checkpoint_chunk").
			Default(0),
		field.Int64("checkpoint_offset").
			Default(0),
		// The id above which a Contact of the source import was made by the batch after the
		// checkpoint: the Workspace's newest Contact when the run began, raised to the newest
		// Contact each batch wrote. A batch run again after a restart counts a Contact its first
		// try made as created, as an uninterrupted run counts it.
		field.Int64("checkpoint_contact_floor").
			Default(0),
		// How many times the periodic sweep queued the run again after its job was gone (a process
		// killed on the job's last try); past contact imports' bound the run fails instead.
		field.Int("recoveries").
			Default(0),
		// What the run has written so far: rows read, Contacts created and updated, rows refused,
		// values dropped, opt-outs and Suppressions recorded.
		field.Int("processed").
			Default(0),
		field.Int("created").
			Default(0),
		field.Int("updated").
			Default(0),
		field.Int("refused").
			Default(0),
		field.Int("dropped").
			Default(0),
		field.Int("opted_out").
			Default(0),
		field.Int("suppressed").
			Default(0),
		// When the run began and ended, written by the machine's moves.
		field.Time("started_at").
			Annotations(Attribute(reasonWithStatus)).
			Optional().
			Nillable(),
		field.Time("ended_at").
			Annotations(Attribute(reasonWithStatus)).
			Optional().
			Nillable(),
		// When a Member asked the running import to stop; the job reads it between batches.
		field.Time("cancel_requested_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
		// When the refused rows' report is deleted if no one downloads it: seven days after the
		// run ended.
		field.Time("report_expires_at").
			Annotations(Attribute(reasonExpiry)).
			Optional().
			Nillable(),
	}
}

func (ContactImport) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("membership", Membership.Type).
			Ref("contact_imports").
			Field("membership_id").
			Unique(),
		// The file and the report die with their import.
		edge.To("chunks", ContactImportChunk.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("report_rows", ContactImportReportRow.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (ContactImport) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id", "membership_id"),
		// The imports of a Workspace in a state: the running one, the uploads to expire.
		index.Fields("workspace_id", "state"),
		// One import runs per Workspace at a time (ADR 0138).
		index.Fields("workspace_id").
			Unique().
			StorageKey("contact_imports_one_running").
			Annotations(entsql.IndexWhere("state = 'running'")),
	}
}

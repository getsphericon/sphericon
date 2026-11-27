package schema

import (
	"encoding/json"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DownloadLink is a one-time link to a file the app writes as it is read (ADR 0138): a Member's
// request names what to download and gets the link, and the browser follows it to the stream. The
// link is bound to the session that asked for it and lives a minute; only the token's hash and the
// session's are stored. Following it deletes the row, so a second follow finds nothing; a link
// never followed is deleted by the next one the Workspace is issued. The payload is what the
// download reads (a Segment and its columns, an import's job), decoded by the module of its
// purpose. Only internal/domain/identity/downloadlinks reads and writes it.
type DownloadLink struct {
	ent.Schema
}

func (DownloadLink) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("download_links", DownloadLink{}.Fields())
	a.Checks["download_links_payload_check"] = jsonCheck("payload", "object")
	return []schema.Annotation{a}
}

func (DownloadLink) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "download_links"}}
}

func (DownloadLink) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// What the link downloads: a CSV export of Contacts, or the refused rows of a CSV import.
		field.Enum("purpose").
			Values("contacts_export", "import_report").
			Immutable(),
		// SHA-256 (hex) of the token the link carries.
		field.String("token_hash").
			NotEmpty().
			Immutable().
			Sensitive(),
		// SHA-256 (hex) of the session token of the request that asked for the link: only that
		// session follows it.
		field.String("session_hash").
			NotEmpty().
			Immutable().
			Sensitive(),
		// What the download reads, a JSON object of its purpose's own shape.
		field.JSON("payload", json.RawMessage{}).
			Immutable(),
		field.Time("expires_at").
			Annotations(Attribute(reasonExpiry)).
			Immutable(),
	}
}

func (DownloadLink) Indexes() []ent.Index {
	return []ent.Index{
		// Following a link finds it by its hash in the Workspace.
		index.Fields("workspace_id", "token_hash").
			Unique().
			StorageKey("download_links_token_hash"),
		// Issuing a link deletes the Workspace's lapsed ones by their expiry.
		index.Fields("workspace_id", "expires_at").
			StorageKey("download_links_workspace_id_expires_at"),
	}
}

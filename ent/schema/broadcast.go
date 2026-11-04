package schema

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/holdreason"
	"github.com/getsphericon/sphericon/internal/telegramrender"
)

// Broadcast is a workspace-scoped one-off send on one Channel, fixed when it is created (ADR
// 0080). Its lifecycle is draft -> scheduled -> sending -> sent (or failed). Its content is the
// Channel's (ADR 0078), and the table checks that exactly that Channel's columns are set: on email
// a subject and a body authored as HTML, whose plain-text part is derived from the final HTML of
// each send and never stored (ADR 0056). Its Segment is the one named by
// segment_id, or all contacts in the workspace when it is unset; leaving draft takes a copy of the
// Segment's Condition, which the plan resolves (ADR 0065).
// Aggregate counters are denormalized for cheap reporting; per-recipient state
// lives in BroadcastRecipient.
type Broadcast struct {
	ent.Schema
}

func (Broadcast) Annotations() []schema.Annotation {
	return []schema.Annotation{
		broadcastTable(),
		Audited{Action: "broadcast", NameField: nameField},
	}
}

func (Broadcast) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "broadcasts"}, ArchiveMixin{}}
}

func (Broadcast) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.String(nameField).
			NotEmpty(),
		// The Channel the Broadcast sends on, set when it is created and never changed: a
		// Broadcast is never converted to another Channel (ADR 0080). Every write sets it; the
		// column default only fills the rows that predate it.
		field.Enum("channel").
			Values("email", "telegram", "sms").
			Immutable().
			Annotations(entsql.Default("email")),
		// Email's content: set on an email Broadcast, unset on any other (broadcastContents).
		field.String("subject").
			Optional().
			Nillable(),
		field.String("from_name").
			Optional().
			Nillable(),
		field.String("from_email").
			Optional().
			Nillable(),
		// Telegram's content, the same Message content as a Telegram step (ADR 0078), set on a
		// Telegram Broadcast and unset on any other (broadcastContents): the text in Telegram's HTML
		// subset with Liquid merge tags, an optional image (the text is then its caption), the link
		// buttons, whether a link unfolds a preview and the label of an optional unsubscribe button.
		// An SMS Broadcast's content is the text alone: plain, with Liquid merge tags.
		field.String("text").
			Optional().
			Nillable(),
		field.String("image_url").
			Optional().
			Nillable(),
		field.JSON("buttons", []telegramrender.Button{}).
			Optional(),
		// Only a Telegram Broadcast reads it; an email Broadcast keeps the default.
		field.Bool("link_preview").
			Default(true),
		field.String("unsubscribe_button").
			Optional().
			Nillable(),
		// Whether the links of an SMS Broadcast's text go out as short tracked links (ADR 0083). Only
		// an SMS Broadcast reads it; any other keeps the default.
		field.Bool("link_tracking").
			Default(true),
		// Email-ready HTML with Liquid merge tags (ADR 0040).
		field.String("body").
			Optional().
			Nillable(),
		// The editor's JSON document (ADR 0040), set only when the Broadcast was written as a
		// document; the server rendered body from it in the same save. A body-only write clears it.
		field.JSON("doc", json.RawMessage{}).
			Optional(),
		// The editor version that rendered doc; set and cleared with it.
		field.String("editor_version").
			Optional().
			Nillable(),
		// Nil segment_id means "all contacts in the workspace".
		field.Int64("segment_id").
			Optional().
			Nillable(),
		// The Segment's Condition, copied when the Broadcast leaves draft (schedule, send) and
		// dropped when it returns to draft (ADR 0065): the plan resolves Recipients from it, so a
		// later edit of the Segment changes nothing for an armed Broadcast, and segment_id stays
		// as provenance. Nil on a draft and on a Broadcast to all contacts; "" is a Segment with
		// no rules yet.
		field.String("condition").
			Optional().
			Nillable(),
		// The Integration of the Broadcast's Channel it sends through; nil means that Channel's
		// default.
		field.Int64("integration_id").
			Optional().
			Nillable(),
		field.Enum("status").
			Values("draft", "scheduled", "sending", "sent", "failed").
			Default("draft"),
		field.Time("scheduled_at").
			Optional().
			Nillable(),
		field.Time("sent_at").
			Optional().
			Nillable(),
		// When the Broadcast failed, set in the write that moves it to failed: Needs attention
		// reads a recent failure from it, so a later edit of the row does not move its age.
		field.Time("failed_at").
			Optional().
			Nillable(),
		// No delivery figures are stored: they are derived from the recipients and their
		// Outbound messages (ADR 0015), so they cannot drift from the rows.
		// Why the broadcast is currently held (ADR 0015): a reversible, per-source
		// hold — Workspace suspension, an unverified Sending domain, no Integration —
		// not a failure. Set while any recipient is held, cleared when one sends; the
		// status stays "sending" and the pending recipients wait for the hold to lift. The
		// reasons are holdreason's, the one list Outbound send holds a send for; a Staff hold's own
		// reason code is never one of them.
		field.Enum("hold_reason").
			GoType(holdreason.Reason("")).
			Optional().
			Nillable(),
		// When the last paced recipient job is scheduled (ADR 0023): the planner spreads
		// recipient jobs at one over the Integration's effective rate, and the ETA is
		// derived from this and the recipients still pending. Nil when the plan was not
		// paced (no Provider limit).
		field.Time("last_scheduled_at").
			Optional().
			Nillable(),
	}
}

func (Broadcast) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("recipients", BroadcastRecipient.Type),
		// A Segment is archived and an Integration discarded, never deleted (ADR 0100), so a
		// Broadcast's reference to one always resolves, as history too. Only Workspace erasure purges them,
		// after the Broadcasts.
		edge.To("segment", Segment.Type).
			Field("segment_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("integration", Integration.Type).
			Field("integration_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (Broadcast) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("workspace_id"),
		index.Fields("segment_id"),
		index.Fields("integration_id"),
	}
}

// broadcastTable adds the timestamps a status promises to the enum checks: a scheduled
// Broadcast has its time, a sent one its sent_at, a failed one its failed_at (each set in the
// write that moves the status). It also checks that the content is exactly the Channel's.
func broadcastTable() entsql.Annotation {
	a := TableWithEnumChecks("broadcasts", Broadcast{}.Fields())
	a.Checks["broadcasts_content_check"] = contentCheck(broadcastContents)
	a.Checks["broadcasts_scheduled_at_check"] = "status <> 'scheduled' OR scheduled_at IS NOT NULL"
	a.Checks["broadcasts_sent_at_check"] = "status <> 'sent' OR sent_at IS NOT NULL"
	a.Checks["broadcasts_failed_at_check"] = "status <> 'failed' OR failed_at IS NOT NULL"
	return a
}

// channelContent names the columns of one Channel's content on the broadcasts table: required
// ones are set on a Broadcast of that Channel, and none of them is set on another Channel's unless
// that Channel's content has the column too (the text of Telegram and SMS).
type channelContent struct {
	channel            string
	required, optional []string
}

// broadcastContents is the content of each Channel a Broadcast is sent on (ADR 0078). A Channel
// added to the channel enum adds its entry here.
var broadcastContents = []channelContent{
	{channel: "email", required: []string{"subject", "body"}, optional: []string{"from_name", "from_email", "doc", "editor_version"}},
	{channel: "telegram", required: []string{"text", "buttons"}, optional: []string{"image_url", "unsubscribe_button"}},
	{channel: "sms", required: []string{"text"}},
}

// contentCheck is the CHECK that a row holds exactly its Channel's content: one arm per Channel,
// each requiring that Channel's required columns and leaving unset every column of another
// Channel's content that is not one of its own.
func contentCheck(contents []channelContent) string {
	arms := make([]string, len(contents))
	for i, c := range contents {
		conds := []string{fmt.Sprintf("channel = '%s'", c.channel)}
		for _, col := range c.required {
			conds = append(conds, col+" IS NOT NULL")
		}
		own := slices.Concat(c.required, c.optional)
		var unset []string
		for _, other := range contents {
			if other.channel == c.channel {
				continue
			}
			for _, col := range slices.Concat(other.required, other.optional) {
				if !slices.Contains(own, col) && !slices.Contains(unset, col) {
					unset = append(unset, col)
				}
			}
		}
		for _, col := range unset {
			conds = append(conds, col+" IS NULL")
		}
		arms[i] = "(" + strings.Join(conds, " AND ") + ")"
	}
	return strings.Join(arms, " OR ")
}

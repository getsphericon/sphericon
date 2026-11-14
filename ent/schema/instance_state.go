package schema

import (
	"crypto/rand"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// InstanceState is the one row an Instance keeps about itself (ADR 0032): its random
// Instance id, the Owner's telemetry setting, whether the Telemetry report notice was
// acknowledged, when a report was last sent, the newest release the Release check
// found and the key that seals public ids. It belongs to the Instance, not a Workspace (no
// Workspace edge, no scoped wrapper); the default Send cap is suspension's (DefaultSendCap). Row
// id 1 is created at first start by internal/instancestates.
type InstanceState struct {
	ent.Schema
}

func (InstanceState) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "instance_state"},
	}
}

func (InstanceState) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (InstanceState) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// InstanceID is derived from nothing: random bytes, generated once.
		field.String("instance_id").
			Immutable().
			DefaultFunc(rand.Text),
		// TelemetryEnabled is the Owner's in-app choice, on until they switch it off
		// (ADR 0032). The environment can override it (see internal/instancestates).
		field.Bool("telemetry_enabled").
			Annotations(Attribute(reasonSetting)).
			Default(true),
		// NoticeAcknowledgedAt is when an Owner acknowledged the one-time notice.
		field.Time("notice_acknowledged_at").
			Annotations(Latch()).
			Optional().
			Nillable(),
		// LastReportSentAt is when the last Telemetry report was sent.
		field.Time("last_report_sent_at").
			Annotations(Attribute(reasonLatest)).
			Optional().
			Nillable(),
		// LatestRelease is the newest release the Release check found (a bare version).
		field.String("latest_release").
			Optional().
			Nillable(),
		// LatestReleaseCheckedAt is when the Release check last succeeded.
		field.Time("latest_release_checked_at").
			Annotations(Attribute(reasonLatest)).
			Optional().
			Nillable(),
		// IDKey seals the primary keys the APIs show (internal/kernel/publicid, ADR 0142): 16 random
		// bytes, made with the row and never changed, since a new key would change every id a
		// client holds. It lives here, not in the environment, so a restored database keeps its ids.
		field.Bytes("id_key").
			Immutable().
			DefaultFunc(idKey).
			Sensitive(),
	}
}

// idKey is a fresh id key.
func idKey() []byte {
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	return key
}

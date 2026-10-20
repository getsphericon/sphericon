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
// acknowledged, when a report was last sent and the newest release the Release check
// found. It belongs to the Instance, not a Workspace (no Workspace edge, no scoped
// wrapper). Row id 1 is created at first start by internal/instance.
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
		// (ADR 0032). The environment can override it (see internal/instance).
		field.Bool("telemetry_enabled").
			Default(true),
		// NoticeAcknowledgedAt is when an Owner acknowledged the one-time notice.
		field.Time("notice_acknowledged_at").
			Optional().
			Nillable(),
		// LastReportSentAt is when the last Telemetry report was sent.
		field.Time("last_report_sent_at").
			Optional().
			Nillable(),
		// LatestRelease is the newest release the Release check found (a bare version).
		field.String("latest_release").
			Optional().
			Nillable(),
		// LatestReleaseCheckedAt is when the Release check last succeeded.
		field.Time("latest_release_checked_at").
			Optional().
			Nillable(),
	}
}

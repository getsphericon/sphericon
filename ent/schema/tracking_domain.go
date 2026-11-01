package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TrackingDomain is a host of the Workspace's own (ADR 0071): a CNAME to the Instance's edge
// host on which the links of the Workspace's mail and the first-party tracker are served, tied to
// no Sending domain. The role says what the host does: the current one is used for new mail and
// the snippet while it serves, a replacement takes over once it serves, and a retired one, which
// served before it was replaced or removed, keeps serving what was already sent and installed on
// it until its CNAME leaves the edge. It is configuration without a history trail (ADR 0049): a
// host that goes is deleted, and internal/trackinghostlog logs it as removed.
type TrackingDomain struct {
	ent.Schema
}

func (TrackingDomain) Annotations() []schema.Annotation {
	return []schema.Annotation{
		trackingDomainTable(),
		Audited{Action: "tracking_domain", NameField: "host"},
	}
}

func (TrackingDomain) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "tracking_domains"}}
}

func (TrackingDomain) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The canonical hostname: lower case, IDN as punycode, e.g. "t.example.com". Another
		// host is another Tracking domain.
		field.String("host").
			NotEmpty().
			Immutable(),
		// What the host does for the Workspace: current (new mail and the snippet use it while
		// it serves), replacement (becomes current once it serves) or retired (serves what was
		// sent and installed on it, nothing new). At most one current and one replacement.
		field.Enum("role").
			Values("current", "replacement", "retired"),
		// pending until a live DNS check finds the CNAME at the Instance's edge host
		// (dns_verified, the edge may now provision), serving once an HTTPS probe of the host
		// succeeds (ADR 0038). A retired host keeps the state it had: only DNS ends it.
		field.Enum("state").
			Values("pending", "dns_verified", "serving").
			Default("pending"),
		// When the host first reached serving (nil = never served): only a host that served is
		// retired rather than deleted, since only it can have links in sent mail.
		field.Time("served_at").
			Optional().
			Nillable(),
		// When the host was most recently checked (nil = never checked yet).
		field.Time("checked_at").
			Optional().
			Nillable(),
		// Why the host is not advancing although its CNAME is right: host_taken means another
		// Workspace holds it. Set by the verifier when the claim loses, cleared by the next check
		// that does not lose.
		field.Enum("reason").
			Values("host_taken").
			Optional().
			Nillable(),
		// When state most recently changed (nil = never changed).
		field.Time("state_changed_at").
			Optional().
			Nillable(),
	}
}

func (TrackingDomain) Indexes() []ent.Index {
	return []ent.Index{
		// A Workspace has a host once, whatever its role.
		index.Fields("workspace_id", "host").
			Unique(),
		// At most one current and one replacement per Workspace. The migration builds it
		// CONCURRENTLY (ADR 0033).
		index.Fields("workspace_id", "role").
			Unique().
			Annotations(entsql.IndexWhere("role IN ('current', 'replacement')")),
		// A host belongs to one Workspace across the Instance among verified rows, retired ones
		// included; pending claims never block anyone. The migration builds it CONCURRENTLY.
		index.Fields("host").
			Unique().
			Annotations(entsql.IndexWhere("state IN ('dns_verified', 'serving')")),
	}
}

// trackingDomainTable adds to the enum checks that a retired host is a verified one: a host that
// lost its DNS goes rather than stays retired.
func trackingDomainTable() entsql.Annotation {
	a := TableWithEnumChecks("tracking_domains", TrackingDomain{}.Fields())
	a.Checks["tracking_domains_retired_check"] = "role <> 'retired' OR state IN ('dns_verified', 'serving')"
	return a
}

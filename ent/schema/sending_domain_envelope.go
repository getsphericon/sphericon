package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// SendingDomainEnvelope is what the last check of a Sending domain read of its envelope at one
// email Integration whose provider keeps it as a setting of its own: SES's custom MAIL FROM of the
// domain's identity (ADR 0184). SPF is checked on the envelope, not on the Sending domain. One row
// per (Sending domain, Integration), written by the check and overwritten whole by the next one;
// an Integration that keeps no such setting (SMTP, the Test inbox) has none.
type SendingDomainEnvelope struct {
	ent.Schema
}

func (SendingDomainEnvelope) Annotations() []schema.Annotation {
	a := TableWithEnumChecks("sending_domain_envelopes", SendingDomainEnvelope{}.Fields())
	a.Checks["sending_domain_envelopes_mail_from_check"] = variantCheck("state", envelopeMailFrom)
	return []schema.Annotation{a}
}

// envelopeMailFrom is the custom MAIL FROM a read found: only a custom envelope has one, whole.
var envelopeMailFrom = []variant{
	{value: "provider_default"},
	{value: "custom", required: []string{"mail_from_domain", "mail_from_status", "mail_from_on_mx_failure"}},
	{value: "unknown"},
}

func (SendingDomainEnvelope) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "sending_domain_envelopes"}}
}

func (SendingDomainEnvelope) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		field.Int64("sending_domain_id").
			Immutable(),
		field.Int64("integration_id").
			Immutable(),
		// What the read found: the provider's own envelope, a custom MAIL FROM, or no answer (no
		// permission, no identity of the domain, a timeout).
		field.Enum("state").
			Values("provider_default", "custom", "unknown").
			Annotations(Attribute(reasonLatest)),
		// The custom MAIL FROM domain set at the provider; only a custom envelope has one.
		field.String("mail_from_domain").
			Optional().
			Nillable(),
		// The provider's status of the custom MAIL FROM domain.
		field.Enum("mail_from_status").
			GoType(providers.MailFromStatus("")).
			Annotations(Attribute(reasonLatest)).
			Optional().
			Nillable(),
		// What the provider does with a send while the custom domain's MX record fails.
		field.Enum("mail_from_on_mx_failure").
			GoType(providers.MXFailureBehavior("")).
			Optional().
			Nillable(),
		// When the envelope was read.
		field.Time("checked_at").
			Annotations(Attribute(reasonLatest)),
	}
}

func (SendingDomainEnvelope) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("sending_domain", SendingDomain.Type).
			Ref("envelopes").
			Field("sending_domain_id").
			Unique().
			Required().
			Immutable(),
		edge.From("integration", Integration.Type).
			Ref("envelopes").
			Field("integration_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (SendingDomainEnvelope) Indexes() []ent.Index {
	return []ent.Index{
		// One read per (Sending domain, Integration); serves the Sending domain's foreign key.
		index.Fields("workspace_id", "sending_domain_id", "integration_id").
			Unique().
			StorageKey("sending_domain_envelopes_workspace_domain_integration"),
		// Serves the Integration's foreign key.
		index.Fields("workspace_id", "integration_id").
			StorageKey("sending_domain_envelopes_workspace_id_integration_id"),
	}
}

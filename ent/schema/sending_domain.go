package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SendingDomain is a workspace-scoped, sphericon-authenticated sending identity
// (ADR 0010). sphericon signs outbound mail itself: it generates a per-domain DKIM
// keypair, the user publishes one <selector>._domainkey TXT, and the same
// verified domain works across every transport (smtp/ses/…) with no
// re-verification when the Integration changes. Distinct from a Sending source
// (the unsubscribe consent scope) despite the similar name.
//
// verified is a *live* property, not a one-time flag: a periodic re-check
// re-validates the DKIM DNS and flips it if the record disappears (see
// internal/jobs). The private key is stored Tink-encrypted; the public key is
// the TXT value and is safe to expose.
type SendingDomain struct {
	ent.Schema
}

func (SendingDomain) Annotations() []schema.Annotation {
	return []schema.Annotation{
		TableWithEnumChecks("sending_domains", SendingDomain{}.Fields()),
		Audited{Action: "sending_domain", NameField: "domain"},
	}
}

func (SendingDomain) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "sending_domains"}}
}

func (SendingDomain) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StorageKey("id").
			Immutable(),
		// The authenticated domain, e.g. "mail.acme.com".
		field.String("domain").
			NotEmpty(),
		// DKIM selector; the DNS record is "<dkim_selector>._domainkey.<domain>".
		field.String("dkim_selector").
			NotEmpty(),
		// Tink-encrypted PKCS#8 PEM private key (internal/secrets.Cipher).
		field.String("dkim_private_key_encrypted").
			Sensitive().
			Annotations(Sensitive{}),
		// The DKIM TXT value ("v=DKIM1; k=rsa; p=<base64 DER>"); safe to expose.
		field.String("dkim_public_key").
			NotEmpty(),
		// Live DKIM-verified state, re-checked by the periodic job.
		field.Bool("verified").
			Default(false),
		// When the DKIM DNS was most recently checked (nil = never checked yet).
		field.Time("last_checked_at").
			Optional().
			Nillable(),
		// When the domain most recently became verified (nil = never verified).
		field.Time("verified_at").
			Optional().
			Nillable(),
		// The state of the Tracking domain record track.<domain> (ADR 0038): pending until
		// a live DNS check finds the CNAME at the Instance's edge host (dns_verified, the
		// edge may now provision), serving once an HTTPS probe of the host succeeds. Mail
		// builds its links on the host only while serving. The host itself is derived.
		field.Enum("tracking_state").
			Values("pending", "dns_verified", "serving").
			Default("pending"),
		// When the Tracking domain was most recently checked (nil = never checked yet).
		field.Time("tracking_checked_at").
			Optional().
			Nillable(),
		// Why the Tracking domain is not advancing although its CNAME is right: host_taken
		// means another Workspace holds the host (ADR 0038). Set by the verifier when the
		// claim loses, cleared by the next check that does not lose.
		field.Enum("tracking_reason").
			Values("host_taken").
			Optional().
			Nillable(),
		// When tracking_state most recently changed (nil = never changed).
		field.Time("tracking_state_changed_at").
			Optional().
			Nillable(),
	}
}

func (SendingDomain) Indexes() []ent.Index {
	return []ent.Index{
		// One row per (workspace, domain).
		index.Fields("workspace_id", "domain").
			Unique(),
		// A Tracking domain host (track.<domain>) belongs to one Workspace across the
		// Instance (ADR 0038): the host is derived from the domain, so the domain is unique
		// among rows whose DNS is verified. Unverified claims never block anyone. The
		// migration builds it CONCURRENTLY (ADR 0033); the diff cannot emit that.
		index.Fields("domain").
			Unique().
			Annotations(entsql.IndexWhere("tracking_state IN ('dns_verified', 'serving')")),
		// Lets the periodic re-check job find the least-recently-checked rows.
		index.Fields("last_checked_at"),
	}
}

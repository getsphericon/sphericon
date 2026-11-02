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
// Verification is kept as moments, not a flag a job flips: verified_at is when the domain
// most recently became verified, verification_lost_at when a re-check found the DKIM DNS gone
// (nil again once it is verified anew). A domain is verified while verified_at is set and
// verification_lost_at is not (internal/dkimstate). The private key is stored Tink-encrypted; the public key is
// the TXT value and is safe to expose.
type SendingDomain struct {
	ent.Schema
}

func (SendingDomain) Annotations() []schema.Annotation {
	return []schema.Annotation{
		sendingDomainTable(),
		Audited{Action: "sending_domain", NameField: "domain"},
	}
}

func (SendingDomain) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, WorkspaceMixin{Ref: "sending_domains"}, DiscardMixin{}}
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
		// When the DKIM DNS was most recently checked (nil = never checked yet).
		field.Time("last_checked_at").
			Optional().
			Nillable(),
		// When the domain most recently became verified (nil = never verified). Kept after a
		// loss, so it is the history of verified.
		field.Time("verified_at").
			Optional().
			Nillable(),
		// When a re-check most recently found the verified DKIM DNS gone (nil = not lost: never
		// verified, or verified again since).
		field.Time("verification_lost_at").
			Optional().
			Nillable(),
		// When a DMARC lookup most recently got an answer, the record found or not (nil = none has:
		// never run, or every lookup failed). Until then nothing is known of the domain's DMARC.
		field.Time("dmarc_checked_at").
			Optional().
			Nillable(),
		// When a lookup first found the domain's DMARC record published, at the domain or its
		// organizational domain (nil = the last lookup found none, or none has answered). It is the
		// bulk-ready signal of ADR 0012 and never gates sending.
		field.Time("dmarc_found_at").
			Optional().
			Nillable(),
	}
}

func (SendingDomain) Indexes() []ent.Index {
	return []ent.Index{
		// One kept row per (workspace, domain): an archived row does not hold the name
		// (ADR 0049). The migration builds it CONCURRENTLY (ADR 0033).
		index.Fields("workspace_id", "domain").
			Unique().
			Annotations(entsql.IndexWhere("discarded_at IS NULL")),
		// Supports the workspace foreign key, which the partial unique index above cannot.
		index.Fields("workspace_id"),
		// Lets the periodic re-check job find the least-recently-checked rows.
		index.Fields("last_checked_at"),
	}
}

// sendingDomainTable adds to the enum checks that a lost verification was a verification:
// verified_at keeps the last time after a loss, so it is the history of verified.
func sendingDomainTable() entsql.Annotation {
	a := TableWithEnumChecks("sending_domains", SendingDomain{}.Fields())
	a.Checks["sending_domains_verification_lost_check"] = "verification_lost_at IS NULL OR verified_at IS NOT NULL"
	return a
}

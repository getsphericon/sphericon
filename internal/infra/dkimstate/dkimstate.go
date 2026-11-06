// Package dkimstate is the one definition of "a Sending domain is verified" (ADR 0010). The
// state is kept as two moments on the row, not a flag: verified_at is when the domain most
// recently became verified, verification_lost_at when a re-check found its DKIM DNS gone. A
// domain is verified while it has a verified_at and no loss since. It is a leaf, so the send
// path, the signer, the setup checklist and the API mapper all read the same rule.
package dkimstate

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
)

// Verified selects the Sending domains that are verified now: the query form of Is.
func Verified() predicate.SendingDomain {
	return sendingdomain.And(sendingdomain.VerifiedAtNotNil(), sendingdomain.VerificationLostAtIsNil())
}

// Is reports whether the row is verified now: the row form of Verified.
func Is(d *ent.SendingDomain) bool {
	return d != nil && d.VerifiedAt != nil && d.VerificationLostAt == nil
}

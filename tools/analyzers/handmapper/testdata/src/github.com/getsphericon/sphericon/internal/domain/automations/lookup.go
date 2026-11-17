package automations

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/gen/site"
)

// A domain module is not an API package: the literal rule does not read it.
func lookup(inv *ent.Invitation) any {
	return site.InvitationLookup{Email: inv.Email}
}

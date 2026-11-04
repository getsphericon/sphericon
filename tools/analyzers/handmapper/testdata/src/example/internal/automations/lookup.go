package automations

import (
	"example/ent"
	"example/gen/site"
)

// A domain module is not an API package: the literal rule does not read it.
func lookup(inv *ent.Invitation) any {
	return site.InvitationLookup{Email: inv.Email}
}

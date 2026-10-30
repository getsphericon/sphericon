// Package roles names the Roles of a Membership for code that needs to know one for something
// other than access: the last owner, whom to mail, raising a Role on an Invitation. It holds the
// only comparisons with a Role outside the Permission table (accounts/permissions.go); an access
// decision asks a Permission instead (ADR 0061). It is a leaf, so the modules that accounts
// itself imports (the workspace feed) can share it.
package roles

import (
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// IsOwner reports whether role is the owner Role.
func IsOwner(role membership.Role) bool { return role == membership.RoleOwner }

// IsAdmin reports whether role is the admin Role.
func IsAdmin(role membership.Role) bool { return role == membership.RoleAdmin }

// IsMember reports whether role is the member Role.
func IsMember(role membership.Role) bool { return role == membership.RoleMember }

// Owners selects Memberships that hold the owner Role.
func Owners() predicate.Membership { return membership.RoleEQ(membership.RoleOwner) }

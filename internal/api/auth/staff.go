package auth

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
)

var staffMemberKey = struct{ name string }{"staffMember"}

// AuthenticateStaff verifies a raw Staff member session token (staffmembers.Sessions.Verify)
// and returns ctx carrying its Staff member, read back with StaffMember: what the security
// handler of every /staff surface does.
func AuthenticateStaff(ctx context.Context, sessions *staffmembers.Sessions, raw string) (context.Context, error) {
	op, err := sessions.Verify(ctx, raw)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, staffMemberKey, op), nil
}

// StaffMember is the Staff member an authenticated /staff request belongs to: the actor, read
// from the context (ADR 0111).
func StaffMember(ctx context.Context) *ent.StaffMember {
	op, _ := ctx.Value(staffMemberKey).(*ent.StaffMember)
	return op
}

package apitokens

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrConnectionNotFound refuses a Connection that is not in the Workspace, is already revoked or
// is another User's while the caller may not see it.
var ErrConnectionNotFound = validation.NotFound("apitokens: connection not found", i18n.ErrorsConnectionNotFound)

// Connections lists the active Connections of the Workspace s that caller (the User's Membership
// of it) may see, oldest first, each with its Membership and that Membership's User: the User's
// own, or every member's when the Role holds members:manage (ADR 0061). That rule is the Policy of
// a Connection, so every surface that lists them gets it.
func Connections(ctx context.Context, s *ent.Scoped, caller *ent.Membership) ([]*ent.ApiToken, error) {
	return s.ApiToken().Query().
		Where(isConnection(), visibleTo(caller), apitoken.RevokedAtIsNil()).
		WithMembership(func(q *ent.MembershipQuery) { q.WithUser() }).
		Order(ent.Asc(apitoken.FieldID)).
		All(ctx)
}

// RevokeConnection retires the active Connection id of the Workspace s on behalf of caller, under
// the Policy of Connections: the caller's own, or anyone's when the Role holds members:manage. A
// Connection the caller may not see is ErrConnectionNotFound, as an unknown one is.
func RevokeConnection(ctx context.Context, s *ent.Scoped, caller *ent.Membership, id int64) error {
	n, err := s.ApiToken().Update().
		Where(apitoken.ID(id), isConnection(), visibleTo(caller), apitoken.RevokedAtIsNil()).
		SetRevokedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

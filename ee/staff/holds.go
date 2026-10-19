package staff

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	enthold "github.com/getsphericon/sphericon/ent/hold"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/holds"
)

// DefaultHoldMaxExpiry is how far ahead a hold's expiry may be set when the instance
// does not say otherwise: a service that stops renewing frees the Workspace by then.
const DefaultHoldMaxExpiry = 30 * 24 * time.Hour

// ErrHoldExpiryInPast refuses a hold that would already have lapsed.
var ErrHoldExpiryInPast = errors.New("staff: a hold's expiry must be in the future")

// HoldTooLongError refuses a hold whose expiry is beyond the configured maximum.
type HoldTooLongError struct{ Max time.Duration }

func (e *HoldTooLongError) Error() string {
	return fmt.Sprintf("staff: a hold's expiry may be at most %s from now", e.Max)
}

// HoldInput is what a setter states about a hold.
type HoldInput struct {
	ReasonCode string
	Message    string
	ExpiresAt  time.Time
}

// HoldChange is the outcome of setting or clearing a hold: the hold concerned (nil when
// a clear found none) and whether the call changed anything.
type HoldChange struct {
	Hold    *ent.Hold
	Changed bool
}

// SetHold sets the hold under key on a Workspace, or renews it when it exists: one call
// does both and repeating it is harmless. It is recorded in the Audit log under the
// credential's name. The expiry is required to be ahead of now and no further than the
// configured maximum. ent.IsNotFound for an unknown Workspace.
//
// The service endpoints scope over the one Workspace the call names, after its row is
// loaded (AGENTS.md, Workspace scoping, list item 7).
func (m *Module) SetHold(ctx context.Context, bus *events.Bus, cred *ent.ServiceCredential, workspaceID int64, key string, in HoldInput) (HoldChange, error) {
	if err := m.licensed(); err != nil {
		return HoldChange{}, err
	}
	now := m.now()
	if !in.ExpiresAt.After(now) {
		return HoldChange{}, ErrHoldExpiryInPast
	}
	if in.ExpiresAt.After(now.Add(m.holdMax)) {
		return HoldChange{}, &HoldTooLongError{Max: m.holdMax}
	}
	s, err := m.holdScope(ctx, bus, cred, workspaceID)
	if err != nil {
		return HoldChange{}, err
	}
	// A concurrent first set of the same key loses the unique index once; the retry
	// then finds the row and renews it.
	for range 2 {
		existing, err := s.Hold().Query().Where(enthold.Key(key)).Only(ctx)
		switch {
		case ent.IsNotFound(err):
			created, err := s.Hold().Create().
				SetKey(key).SetReasonCode(in.ReasonCode).SetMessage(in.Message).
				SetExpiresAt(in.ExpiresAt).SetSetBy(cred.Name).
				Save(ctx)
			if ent.IsConstraintError(err) {
				continue
			}
			return HoldChange{Hold: created, Changed: err == nil}, err
		case err != nil:
			return HoldChange{}, err
		}
		if sameHold(existing, in) {
			return HoldChange{Hold: existing}, nil
		}
		updated, err := s.Hold().UpdateOneID(existing.ID).
			SetReasonCode(in.ReasonCode).SetMessage(in.Message).
			SetExpiresAt(in.ExpiresAt).SetSetBy(cred.Name).
			Save(ctx)
		return HoldChange{Hold: updated, Changed: err == nil}, err
	}
	return HoldChange{}, errors.New("staff: could not set the hold: it kept changing under us")
}

// sameHold reports whether the stored hold already says what in says. Timestamps are
// compared at the database's microsecond precision.
func sameHold(h *ent.Hold, in HoldInput) bool {
	return h.ReasonCode == in.ReasonCode && h.Message == in.Message &&
		h.ExpiresAt.Truncate(time.Microsecond).Equal(in.ExpiresAt.Truncate(time.Microsecond))
}

// ClearHold removes the hold under key. Clearing a missing hold succeeds. It is recorded
// in the Audit log under the credential's name. ent.IsNotFound for an unknown Workspace.
func (m *Module) ClearHold(ctx context.Context, bus *events.Bus, cred *ent.ServiceCredential, workspaceID int64, key string) (HoldChange, error) {
	if err := m.licensed(); err != nil {
		return HoldChange{}, err
	}
	s, err := m.holdScope(ctx, bus, cred, workspaceID)
	if err != nil {
		return HoldChange{}, err
	}
	existing, err := s.Hold().Query().Where(enthold.Key(key)).Only(ctx)
	if ent.IsNotFound(err) {
		return HoldChange{}, nil
	}
	if err != nil {
		return HoldChange{}, err
	}
	err = s.Hold().DeleteOneID(existing.ID).Exec(ctx)
	if ent.IsNotFound(err) {
		return HoldChange{}, nil
	}
	return HoldChange{Hold: existing, Changed: err == nil}, err
}

// ActiveHolds lists the holds of a Workspace that are active now, by key. ent.IsNotFound
// for an unknown Workspace.
func (m *Module) ActiveHolds(ctx context.Context, workspaceID int64) ([]*ent.Hold, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return holds.Active(ctx, s, m.now())
}

// holdScope loads the Workspace row and scopes over it, acting as the credential.
func (m *Module) holdScope(ctx context.Context, bus *events.Bus, cred *ent.ServiceCredential, workspaceID int64) (*ent.Scoped, error) {
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return bus.Act(s, ServiceActor(cred)), nil
}

// ServiceHoldsList lists a Workspace's active holds.
func (h *ServiceHandlers) ServiceHoldsList(ctx context.Context, params serviceapi.ServiceHoldsListParams) (serviceapi.ServiceHoldsListRes, error) {
	id := int64(params.WorkspaceId)
	rows, err := h.module.ActiveHolds(ctx, id)
	if ent.IsNotFound(err) {
		p := serviceapi.ServiceHoldsListNotFound(workspaceNotFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]serviceapi.HoldResource, len(rows))
	for i, r := range rows {
		items[i] = holdResource(r)
	}
	return &serviceapi.HoldList{Items: items}, nil
}

// ServiceHoldsSet sets or renews a hold.
func (h *ServiceHandlers) ServiceHoldsSet(ctx context.Context, req *serviceapi.HoldInput, params serviceapi.ServiceHoldsSetParams) (serviceapi.ServiceHoldsSetRes, error) {
	id := int64(params.WorkspaceId)
	change, err := h.module.SetHold(ctx, h.bus, CurrentService(ctx), id, params.Key, HoldInput{
		ReasonCode: req.ReasonCode, Message: req.Message, ExpiresAt: time.Time(req.ExpiresAt),
	})
	var tooLong *HoldTooLongError
	switch {
	case ent.IsNotFound(err):
		p := serviceapi.ServiceHoldsSetNotFound(workspaceNotFoundProblem())
		return &p, nil
	case errors.Is(err, ErrHoldExpiryInPast), errors.As(err, &tooLong):
		p := serviceapi.ServiceHoldsSetUnprocessableEntity{
			Status: serviceapi.NewOptInt32(http.StatusUnprocessableEntity),
			Title:  serviceapi.NewOptString(http.StatusText(http.StatusUnprocessableEntity)),
			Detail: serviceapi.NewOptString(err.Error()),
		}
		return &p, nil
	case err != nil:
		return nil, err
	}
	return &serviceapi.HoldSetResult{Changed: change.Changed, Hold: holdResource(change.Hold)}, nil
}

// ServiceHoldsClear clears a hold; a missing one is cleared already.
func (h *ServiceHandlers) ServiceHoldsClear(ctx context.Context, params serviceapi.ServiceHoldsClearParams) (serviceapi.ServiceHoldsClearRes, error) {
	id := int64(params.WorkspaceId)
	change, err := h.module.ClearHold(ctx, h.bus, CurrentService(ctx), id, params.Key)
	if ent.IsNotFound(err) {
		p := serviceapi.ServiceHoldsClearNotFound(workspaceNotFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldClearResult{Changed: change.Changed}, nil
}

func holdResource(r *ent.Hold) serviceapi.HoldResource {
	return serviceapi.HoldResource{
		Key: r.Key, ReasonCode: r.ReasonCode, Message: r.Message, SetBy: r.SetBy,
		ExpiresAt: serviceapi.Timestamp(r.ExpiresAt),
		CreatedAt: serviceapi.Timestamp(r.CreatedAt),
		UpdatedAt: serviceapi.Timestamp(r.UpdatedAt),
	}
}

func workspaceNotFoundProblem() serviceapi.ProblemDetails {
	return serviceapi.ProblemDetails{
		Status: serviceapi.NewOptInt32(http.StatusNotFound),
		Title:  serviceapi.NewOptString(http.StatusText(http.StatusNotFound)),
		Detail: serviceapi.NewOptString("workspace not found"),
	}
}

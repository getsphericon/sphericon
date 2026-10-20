package staff

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// ErrHoldActionURL refuses an action URL that is not an absolute https URL: it is shown
// to the Workspace's Owners as a link, so no other scheme and no credentials in it.
var ErrHoldActionURL = errors.New("staff: a hold's action URL must be an absolute https URL")

// HoldTooLongError refuses a hold whose expiry is beyond the configured maximum.
type HoldTooLongError struct{ Max time.Duration }

func (e *HoldTooLongError) Error() string {
	return fmt.Sprintf("staff: a hold's expiry may be at most %s from now", e.Max)
}

// HoldInput is what a setter states about a hold.
type HoldInput struct {
	ReasonCode string
	Message    string
	// ActionURL is where the Workspace's Owners and Admins can resolve the hold; nil for none.
	ActionURL *string
	ExpiresAt time.Time
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
func (m *Module) SetHold(ctx context.Context, bus *events.Bus, cred *ent.ServiceCredential, workspaceID string, key string, in HoldInput) (HoldChange, error) {
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
	if in.ActionURL != nil {
		if u, err := url.Parse(*in.ActionURL); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return HoldChange{}, ErrHoldActionURL
		}
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
				SetKey(key).SetReasonCode(in.ReasonCode).SetMessage(in.Message).SetNillableActionURL(in.ActionURL).
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
		update := s.Hold().UpdateOneID(existing.ID).
			SetReasonCode(in.ReasonCode).SetMessage(in.Message).
			SetExpiresAt(in.ExpiresAt).SetSetBy(cred.Name)
		if in.ActionURL != nil {
			update.SetActionURL(*in.ActionURL)
		} else {
			update.ClearActionURL()
		}
		updated, err := update.Save(ctx)
		return HoldChange{Hold: updated, Changed: err == nil}, err
	}
	return HoldChange{}, errors.New("staff: could not set the hold: it kept changing under us")
}

// sameHold reports whether the stored hold already says what in says. Timestamps are
// compared at the database's microsecond precision.
func sameHold(h *ent.Hold, in HoldInput) bool {
	return h.ReasonCode == in.ReasonCode && h.Message == in.Message && equalStrings(h.ActionURL, in.ActionURL) &&
		h.ExpiresAt.Truncate(time.Microsecond).Equal(in.ExpiresAt.Truncate(time.Microsecond))
}

func equalStrings(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ClearHold removes the hold under key. Clearing a missing hold succeeds. It is recorded
// in the Audit log under the credential's name. ent.IsNotFound for an unknown Workspace.
func (m *Module) ClearHold(ctx context.Context, bus *events.Bus, cred *ent.ServiceCredential, workspaceID string, key string) (HoldChange, error) {
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
func (m *Module) ActiveHolds(ctx context.Context, workspaceID string) ([]*ent.Hold, error) {
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
func (m *Module) holdScope(ctx context.Context, bus *events.Bus, cred *ent.ServiceCredential, workspaceID string) (*ent.Scoped, error) {
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return bus.Act(s, ServiceActor(cred)), nil
}

// ServiceHoldsList lists a Workspace's active holds.
func (h *ServiceHandlers) ServiceHoldsList(ctx context.Context, params serviceapi.ServiceHoldsListParams) (serviceapi.ServiceHoldsListRes, error) {
	id := string(params.WorkspaceId)
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
	id := string(params.WorkspaceId)
	change, err := h.module.SetHold(ctx, h.bus, CurrentService(ctx), id, params.Key, HoldInput{
		ReasonCode: req.ReasonCode, Message: req.Message, ActionURL: optNilString(req.ActionUrl), ExpiresAt: time.Time(req.ExpiresAt),
	})
	var tooLong *HoldTooLongError
	switch {
	case ent.IsNotFound(err):
		p := serviceapi.ServiceHoldsSetNotFound(workspaceNotFoundProblem())
		return &p, nil
	case errors.Is(err, ErrHoldExpiryInPast), errors.Is(err, ErrHoldActionURL), errors.As(err, &tooLong):
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
	id := string(params.WorkspaceId)
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
		ActionUrl: serviceNilableString(r.ActionURL),
		ExpiresAt: serviceapi.Timestamp(r.ExpiresAt),
		CreatedAt: serviceapi.Timestamp(r.CreatedAt),
		UpdatedAt: serviceapi.Timestamp(r.UpdatedAt),
	}
}

func optNilString(o serviceapi.OptNilString) *string {
	if v, ok := o.Get(); ok {
		return &v
	}
	return nil
}

func serviceNilableString(v *string) serviceapi.OptNilString {
	if v == nil {
		return serviceapi.OptNilString{}
	}
	return serviceapi.NewOptNilString(*v)
}

func workspaceNotFoundProblem() serviceapi.ProblemDetails {
	return serviceapi.ProblemDetails{
		Status: serviceapi.NewOptInt32(http.StatusNotFound),
		Title:  serviceapi.NewOptString(http.StatusText(http.StatusNotFound)),
		Detail: serviceapi.NewOptString("workspace not found"),
	}
}

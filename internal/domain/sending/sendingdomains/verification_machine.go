package sendingdomains

import (
	"context"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// DKIMState is a Sending domain's DKIM verification (ADR 0010), a machine (ADR 0109) kept as its
// two moments with no column of its own: dkimstate is the one read rule, and DKIMStateOf derives
// the state from it.
type DKIMState string

const (
	// DKIMUnverified has never been verified.
	DKIMUnverified DKIMState = "unverified"
	// DKIMVerified signs and sends.
	DKIMVerified DKIMState = "verified"
	// DKIMLost was verified, and a check found its record gone or its selector moved.
	DKIMLost DKIMState = "lost"
)

// DKIMTrigger moves the verification.
type DKIMTrigger string

const (
	// DKIMFound is a check that found the published key under the selector.
	DKIMFound DKIMTrigger = "found"
	// DKIMMissing is a check that did not.
	DKIMMissing DKIMTrigger = "missing"
	// DKIMSelectorMoved is the Owner's edit of the selector: the published record is under the old
	// one, so a verified domain loses its verification at once. It is the Owner's own act, so no
	// Owner is told.
	DKIMSelectorMoved DKIMTrigger = "selector moved"
)

// DKIMStateOf derives the state of the row through dkimstate.
func DKIMStateOf(d *ent.SendingDomain) DKIMState {
	switch {
	case dkimstate.Is(d):
		return DKIMVerified
	case d.VerifiedAt != nil:
		return DKIMLost
	default:
		return DKIMUnverified
	}
}

// DKIMMachine is the verification's table over storage. A check that finds what the state already
// says moves nothing, and a selector move of a domain that is not verified is a plain edit.
func DKIMMachine(storage lifecycle.Storage[DKIMState]) *stateless.StateMachine {
	m := lifecycle.New(storage)
	m.Configure(DKIMUnverified).
		Permit(DKIMFound, DKIMVerified).
		Ignore(DKIMMissing).
		Ignore(DKIMSelectorMoved)
	m.Configure(DKIMVerified).
		Permit(DKIMMissing, DKIMLost).
		Permit(DKIMSelectorMoved, DKIMLost).
		Ignore(DKIMFound)
	m.Configure(DKIMLost).
		Permit(DKIMFound, DKIMVerified).
		Ignore(DKIMMissing).
		Ignore(DKIMSelectorMoved)
	return m
}

// asJudged is the condition that dom is still the domain a check or an edit read: kept, with the
// same moments, selector and key. A check whose selector or key moved meanwhile is stale and
// changes nothing.
func asJudged(dom *ent.SendingDomain) []predicate.SendingDomain {
	at := []predicate.SendingDomain{
		sendingdomain.ID(dom.ID),
		sendingdomain.DiscardedAtIsNil(),
		sendingdomain.DkimSelector(dom.DkimSelector),
		sendingdomain.DkimPublicKey(dom.DkimPublicKey),
	}
	if dom.VerifiedAt == nil {
		at = append(at, sendingdomain.VerifiedAtIsNil())
	} else {
		at = append(at, sendingdomain.VerifiedAtEQ(*dom.VerifiedAt))
	}
	if dom.VerificationLostAt == nil {
		at = append(at, sendingdomain.VerificationLostAtIsNil())
	} else {
		at = append(at, sendingdomain.VerificationLostAtEQ(*dom.VerificationLostAt))
	}
	return at
}

// moveDKIM fires trigger on the verification of dom as it was read, writing through s at now; also
// adds what the move writes besides the moments (the check time, the new selector). It reports
// whether the state moved; a lost swap is lifecycle.ErrInvalidTransition.
func moveDKIM(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain, trigger DKIMTrigger, now time.Time, also func(*ent.SendingDomainScopedUpdate)) (bool, error) {
	moved := false
	machine := DKIMMachine(lifecycle.Storage[DKIMState]{
		Read: func(context.Context) (DKIMState, error) { return DKIMStateOf(dom), nil },
		Write: func(ctx context.Context, _, to DKIMState) (bool, error) {
			upd := s.SendingDomain().Update().Where(asJudged(dom)...)
			if to == DKIMVerified {
				upd.SetVerifiedAt(now).ClearVerificationLostAt()
			} else {
				upd.SetVerificationLostAt(now)
			}
			also(upd)
			n, err := upd.Save(ctx)
			moved = n == 1
			return moved, err
		},
	})
	err := machine.FireCtx(ctx, trigger)
	return moved, err
}

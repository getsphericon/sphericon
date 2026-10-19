// Package trackingdomain verifies a Sending domain's Tracking domain (ADR 0038): a live
// DNS check that track.<domain> is a CNAME to the Instance's edge host, and the state
// that check keeps (pending -> dns_verified). The serving step (an HTTPS probe) and the
// periodic recheck build on Check.
//
// The resolver and the clock are injected, so nothing here reads the network or the wall
// clock. A host is unique across the Instance among rows in dns_verified or serving; the
// partial unique index on the domain enforces it, and Check reports a lost race as
// ErrHostTaken by catching the violation, never by looking first.
package trackingdomain

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// ErrHostTaken means another Workspace holds the host in dns_verified or serving: this
// claim stays pending however correct its DNS is.
var ErrHostTaken = errors.New("tracking host is verified by another workspace")

// CNAMELookup resolves the canonical name of a host. net.Resolver.LookupCNAME satisfies
// it; a host with no CNAME answers itself or a not-found error.
type CNAMELookup func(ctx context.Context, host string) (string, error)

// Verifier checks Tracking domains against Target, the edge host the CNAME must reach.
type Verifier struct {
	Lookup CNAMELookup
	Target string
	Now    func() time.Time
}

// Check re-verifies one Sending domain's Tracking domain from live DNS and stores the
// outcome: the check time always, and the state when it changes. A CNAME at Target moves
// pending to dns_verified (serving stays serving); anything else sends the state back to
// pending. A resolver failure other than not-found returns its error and changes nothing.
// It returns the resulting state, with ErrHostTaken when the host is held elsewhere.
func (v *Verifier) Check(ctx context.Context, s *ent.Scoped, id int64) (sendingdomain.TrackingState, error) {
	row, err := s.SendingDomain().Get(ctx, id)
	if err != nil {
		return "", err
	}
	ok, err := v.pointsAtTarget(ctx, sendingdomains.TrackingHost(row.Domain))
	if err != nil {
		return "", err
	}

	now := v.Now()
	next := sendingdomain.TrackingStatePending
	if ok {
		next = row.TrackingState
		if next == sendingdomain.TrackingStatePending {
			next = sendingdomain.TrackingStateDNSVerified
		}
	}

	upd := s.SendingDomain().UpdateOneID(id).SetTrackingCheckedAt(now)
	if next != row.TrackingState {
		upd.SetTrackingState(next).SetTrackingStateChangedAt(now)
	}
	err = upd.Exec(ctx)
	if db.IsUniqueViolation(err) {
		// Another Workspace verified the host first. Record the check, keep the state.
		if cerr := s.SendingDomain().UpdateOneID(id).SetTrackingCheckedAt(now).Exec(ctx); cerr != nil {
			return "", cerr
		}
		return row.TrackingState, ErrHostTaken
	}
	if err != nil {
		return "", err
	}
	return next, nil
}

// pointsAtTarget reports whether host's CNAME resolves to Target. A missing record is
// false with a nil error; so is an answer of the host itself (no CNAME).
func (v *Verifier) pointsAtTarget(ctx context.Context, host string) (bool, error) {
	cname, err := v.Lookup(ctx, host)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return normalize(cname) == normalize(v.Target) && normalize(cname) != normalize(host), nil
}

func normalize(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

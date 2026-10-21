// Package trackingdomain verifies a Sending domain's Tracking domain (ADR 0038): a live
// DNS check that track.<domain> is a CNAME to the Instance's edge host, and the state
// that check keeps (pending -> dns_verified), and the HTTPS probe that moves a verified
// host to serving. The periodic job calls Check for every row.
//
// The resolver and the clock are injected, so nothing here reads the network or the wall
// clock. A host is unique across the Instance among rows in dns_verified or serving; the
// partial unique index on the domain enforces it, and Check reports a lost race as
// ErrHostTaken by catching the violation, never by looking first. The state moves through a
// lifecycle table (ADR 0046) whose write is a compare-and-swap from the state Check read; a
// check that lost it returns lifecycle.ErrInvalidTransition and stores nothing.
package trackingdomain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// ErrHostTaken means another Workspace holds the host in dns_verified or serving: this
// claim stays pending however correct its DNS is.
var ErrHostTaken = errors.New("tracking host is verified by another workspace")

// CNAMELookup resolves the canonical name of a host. net.Resolver.LookupCNAME satisfies
// it; a host with no CNAME answers itself or a not-found error.
type CNAMELookup func(ctx context.Context, host string) (string, error)

// Doer is the HTTP client the probe uses: the SSRF-guarded one in production.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Verifier checks Tracking domains against Target, the edge host the CNAME must reach.
// With HTTP and Secret set it also proves serving: a host reaches serving only when the
// HTTPS probe answers ProbeValue(Secret, host). Without HTTP no host ever becomes serving.
type Verifier struct {
	Lookup CNAMELookup
	Target string
	Now    func() time.Time
	HTTP   Doer
	Secret string
	Events *events.Bus
}

// Check re-verifies one Sending domain's Tracking domain from live DNS and, once the
// CNAME is right, the HTTPS probe, and stores the outcome: the check time always, and
// the state when it changes, with a TrackingDomainChanged event in the same transaction.
// DNS gone sends the state back to pending; a CNAME at Target with a failing probe is
// dns_verified (a serving host falls back to it); a passing probe is serving. A resolver
// failure other than not-found returns its error and changes nothing. It returns the
// resulting state, with ErrHostTaken when the host is held elsewhere; the row then carries
// the reason host_taken until a later check stores its outcome without losing the claim
// (the reason is cleared on every stored outcome, including an absent CNAME).
func (v *Verifier) Check(ctx context.Context, s *ent.Scoped, id int64) (sendingdomain.TrackingState, error) {
	row, err := s.SendingDomain().Get(ctx, id)
	if err != nil {
		return "", err
	}
	host := sendingdomains.TrackingHost(row.Domain)
	ok, err := v.pointsAtTarget(ctx, host)
	if err != nil {
		return "", err
	}

	now := v.Now()
	next := sendingdomain.TrackingStatePending
	if ok {
		next = sendingdomain.TrackingStateDNSVerified
		switch {
		case v.HTTP == nil:
			// No way to prove serving: keep what the row has, never promote.
			if row.TrackingState != sendingdomain.TrackingStatePending {
				next = row.TrackingState
			}
		case v.probe(ctx, host):
			next = sendingdomain.TrackingStateServing
		}
	}

	err = v.store(ctx, s, row, next, now)
	if db.IsUniqueViolation(err) {
		// Another Workspace verified the host first. Record the check and why, keep the state.
		cerr := s.SendingDomain().UpdateOneID(id).
			SetTrackingCheckedAt(now).SetTrackingReason(sendingdomain.TrackingReasonHostTaken).Exec(ctx)
		if cerr != nil {
			return "", cerr
		}
		return row.TrackingState, ErrHostTaken
	}
	if err != nil {
		return "", err
	}
	return next, nil
}

func (v *Verifier) store(ctx context.Context, s *ent.Scoped, row *ent.SendingDomain, next sendingdomain.TrackingState, now time.Time) error {
	if next == row.TrackingState {
		return s.SendingDomain().UpdateOneID(row.ID).SetTrackingCheckedAt(now).ClearTrackingReason().Exec(ctx)
	}
	return v.Events.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		if err := sendingdomains.TrackingMachine(ts, row, now).FireCtx(ctx, next); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.TrackingDomainChanged{
			WorkspaceID: row.WorkspaceID, SendingDomainID: row.ID,
			Host: sendingdomains.TrackingHost(row.Domain),
			From: string(row.TrackingState), To: string(next),
		})
	})
}

// probe reports whether https://host/ProbePath answers 200 with the host's ProbeValue.
// Any failure, an error, another status or another body, is a failed probe.
func (v *Verifier) probe(ctx context.Context, host string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+ProbePath, nil)
	if err != nil {
		return false
	}
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) == ProbeValue(v.Secret, host)
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

// ProbePath is the well-known path a Tracking host serves for the serving check
// (ADR 0038): GET https://<host>/.well-known/sphericon-tracking answers 200 with
// ProbeValue(secret, host) as the plain-text body. The probe succeeds only when the
// body equals the value for the host it asked, so an answer from another host, a
// parked page or a proxy that is not ours does not pass.
const ProbePath = "/.well-known/sphericon-tracking"

// ProbeValue is the body ProbePath answers for host: a hex HMAC-SHA256 of the lowercase
// host under secret (the Instance JWT secret), so it is tied to the domain and cannot
// be guessed or copied from another host. Both sides compute it; nothing is stored.
func ProbeValue(secret, host string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("tracking-probe:" + normalize(host)))
	return hex.EncodeToString(mac.Sum(nil))
}

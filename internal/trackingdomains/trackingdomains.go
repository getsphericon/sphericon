// Package trackingdomains is the module for a Workspace's Tracking domains (ADR 0038, ADR 0071):
// a host of the Workspace's own, a CNAME to the Instance's edge, on which the links of its mail and
// the first-party tracker are served. /site and /api are thin adapters over it.
//
// A Workspace uses one host at a time (the current one), may have a replacement that takes over once
// it serves, and keeps retired hosts serving what was sent and installed on them until their CNAME
// leaves the edge or the Owner forgets them. Set, Remove, Verify and the job's Recheck are the only
// ways in; the role and the state each move through a lifecycle table (ADR 0046) whose write is a
// compare-and-swap.
//
// It holds the raw client for the job's side only: Due lists the verified hosts of every Workspace
// and Recheck loads a row by id before any scope exists. Every write goes through the Workspace's
// scope. The resolver, the probe client and the clock are injected, so nothing here reads the
// network or the wall clock by itself.
package trackingdomains

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/qmuntal/stateless"
	"golang.org/x/net/publicsuffix"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/ownernotice"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/trackinghostlog"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The refusals of the module, one per meaning.
var (
	// ErrOff means the Instance offers no Tracking domains (no CNAME target is configured).
	ErrOff = validation.NotFound("trackingdomains: the instance offers no tracking domains", i18n.ErrorsTrackingDomainsOff)
	// ErrNotFound means no Tracking domain of that id is in the Workspace.
	ErrNotFound = validation.NotFound("trackingdomains: tracking domain not found", i18n.ErrorsTrackingDomainNotFound)
	// ErrInvalidHost means the host is not a name below a registrable domain.
	ErrInvalidHost = validation.RefuseField("host", "trackingdomains: not a valid host", i18n.ErrorsTrackingDomainInvalid)
	// ErrOwnHost means the host is one of the Instance's own.
	ErrOwnHost = validation.RefuseField("host", "trackingdomains: the host is the instance's own", i18n.ErrorsTrackingDomainOwnHost)
)

func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// CNAMELookup resolves the canonical name of a host. net.Resolver.LookupCNAME satisfies
// it; a host with no CNAME answers itself or a not-found error.
type CNAMELookup func(ctx context.Context, host string) (string, error)

// Doer is the HTTP client the probe uses: the SSRF-guarded one in production.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Checker proves a host: its CNAME reaches Target, and with HTTP set the HTTPS probe answers
// ProbeValue(Secret, host). Without HTTP no host ever becomes serving.
type Checker struct {
	Lookup CNAMELookup
	Target string
	HTTP   Doer
	Secret string
}

// Config is what the module needs besides its collaborators. A nil Checker turns the feature off.
type Config struct {
	Checker *Checker
	// OwnHosts are the Instance's own hosts (the app, the tracker), never a Tracking domain.
	OwnHosts []string
	Now      func() time.Time
}

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	client   *ent.Client
	bus      *events.Bus
	notify   ownernotice.Notify
	check    *Checker
	ownHosts []string
	now      func() time.Time
}

func New(client *ent.Client, bus *events.Bus, notify ownernotice.Notify, cfg Config) *Module {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	own := make([]string, 0, len(cfg.OwnHosts)+1)
	for _, h := range append(slices.Clone(cfg.OwnHosts), targetOf(cfg.Checker)) {
		if h = normalize(h); h != "" {
			own = append(own, h)
		}
	}
	return &Module{client: client, bus: bus, notify: notify, check: cfg.Checker, ownHosts: own, now: now}
}

func targetOf(c *Checker) string {
	if c == nil {
		return ""
	}
	return c.Target
}

// Target is the edge host a Tracking domain's CNAME points at, empty while the feature is off.
func (m *Module) Target() string { return targetOf(m.check) }

// Record is the CNAME a Tracking domain's host needs: Host points at Target.
type Record struct {
	Host   string
	Target string
}

// Record is the CNAME to publish for the Tracking domain.
func (m *Module) Record(d *ent.TrackingDomain) Record {
	return Record{Host: d.Host, Target: m.Target()}
}

// List returns the Workspace's Tracking domains: the current one, the replacement, then the
// retired ones, each group by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped) ([]*ent.TrackingDomain, error) {
	rows, err := s.TrackingDomain().Query().Order(ent.Asc(trackingdomain.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(rows, func(a, b *ent.TrackingDomain) int { return roleRank(a.Role) - roleRank(b.Role) })
	return rows, nil
}

func roleRank(r trackingdomain.Role) int {
	switch r {
	case trackingdomain.RoleCurrent:
		return 0
	case trackingdomain.RoleReplacement:
		return 1
	default:
		return 2
	}
}

// Get returns the Workspace's Tracking domain with the id; an unknown id is ErrNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.TrackingDomain, error) {
	row, err := s.TrackingDomain().Get(ctx, id)
	return row, notFound(err)
}

// ServingHost is the host the Workspace's new mail and snippet are built on: the current Tracking
// domain while it serves, "" otherwise (the Instance's hosts are used then).
func (m *Module) ServingHost(ctx context.Context, s *ent.Scoped) (string, error) {
	row, err := s.TrackingDomain().Query().
		Where(trackingdomain.RoleEQ(trackingdomain.RoleCurrent), trackingdomain.StateEQ(trackingdomain.StateServing)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return row.Host, nil
}

// CanonicalHost validates raw as a Tracking domain host and returns its canonical form: lower
// case, IDN as punycode, below a registrable domain (never the apex, where a CNAME cannot sit).
func CanonicalHost(raw string) (string, bool) {
	host, ok := sending.NormalizeDomain(raw)
	if !ok {
		return "", false
	}
	apex, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil || apex == host {
		return "", false
	}
	return host, true
}

// Set makes host the Workspace's Tracking domain (ADR 0071). With no current host it becomes the
// current one. While the current host serves it becomes the replacement, discarding a pending one,
// and takes over once it serves. When the current host does not serve, the new one replaces it at
// once (the old one is retired when it ever served, deleted otherwise). A host the Workspace
// already uses is returned as it is; a retired one is brought back in the same way as a new one,
// keeping its state, so a host that still serves takes over at once. It returns the row of host.
func (m *Module) Set(ctx context.Context, s *ent.Scoped, raw string) (*ent.TrackingDomain, error) {
	if m.check == nil {
		return nil, ErrOff
	}
	host, ok := CanonicalHost(raw)
	if !ok {
		return nil, ErrInvalidHost
	}
	if slices.Contains(m.ownHosts, host) {
		return nil, ErrOwnHost
	}
	var (
		row      *ent.TrackingDomain
		promoted *ent.TrackingDomain
		retired  *ent.TrackingDomain
	)
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		rows, err := ts.TrackingDomain().Query().All(ctx)
		if err != nil {
			return err
		}
		cur, rep := byRole(rows, trackingdomain.RoleCurrent), byRole(rows, trackingdomain.RoleReplacement)
		existing := byHost(rows, host)
		if existing != nil && existing.Role != trackingdomain.RoleRetired {
			row = existing
			return nil
		}
		if rep != nil {
			if err := ts.TrackingDomain().DeleteOneID(rep.ID).Exec(ctx); err != nil {
				return err
			}
		}
		role := trackingdomain.RoleCurrent
		switch {
		case cur != nil && cur.State == trackingdomain.StateServing:
			role = trackingdomain.RoleReplacement
		case cur != nil:
			if err := m.stepDown(ctx, ts, cur); err != nil {
				return err
			}
		}
		if existing != nil {
			if err := roleMachine(ts, existing).FireCtx(ctx, role); err != nil {
				return err
			}
		} else if err := ts.TrackingDomain().Create().SetHost(host).SetRole(role).Exec(ctx); err != nil {
			return err
		}
		row, err = ts.TrackingDomain().Query().Where(trackingdomain.Host(host)).Only(ctx)
		if err != nil {
			return err
		}
		if role == trackingdomain.RoleReplacement && row.State == trackingdomain.StateServing {
			// A retired host brought back that still serves takes over at once.
			promoted, retired, err = m.promote(ctx, ts, row)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if promoted != nil {
		m.notifyReplaced(ctx, promoted, retired)
		return promoted, nil
	}
	return row, nil
}

// Remove takes the Tracking domain out by its role: the current one stops being used (retired when
// it served, deleted otherwise) and a pending replacement becomes current; a replacement is
// cancelled; a retired host is forgotten, so links sent on it stop working. An unknown id is
// ErrNotFound.
func (m *Module) Remove(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := ts.TrackingDomain().Get(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if row.Role != trackingdomain.RoleCurrent {
			return ts.TrackingDomain().DeleteOneID(id).Exec(ctx)
		}
		if err := m.stepDown(ctx, ts, row); err != nil {
			return err
		}
		rep, err := ts.TrackingDomain().Query().Where(trackingdomain.RoleEQ(trackingdomain.RoleReplacement)).Only(ctx)
		if ent.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return roleMachine(ts, rep).FireCtx(ctx, trackingdomain.RoleCurrent)
	})
}

// stepDown takes a current host out of use: retired when it served and is still verified, so the
// links sent on it keep working, deleted otherwise.
func (m *Module) stepDown(ctx context.Context, ts *ent.Scoped, row *ent.TrackingDomain) error {
	if row.ServedAt != nil && trackinghostlog.Serves(row) {
		return roleMachine(ts, row).FireCtx(ctx, trackingdomain.RoleRetired)
	}
	return ts.TrackingDomain().DeleteOneID(row.ID).Exec(ctx)
}

// promote makes a serving replacement the current host, stepping the current one down. It returns
// the new current row and the one it replaced (nil when there was none).
func (m *Module) promote(ctx context.Context, ts *ent.Scoped, rep *ent.TrackingDomain) (now, replaced *ent.TrackingDomain, err error) {
	replaced, err = ts.TrackingDomain().Query().Where(trackingdomain.RoleEQ(trackingdomain.RoleCurrent)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, nil, err
	}
	if replaced != nil {
		if err := m.stepDown(ctx, ts, replaced); err != nil {
			return nil, nil, err
		}
	}
	if err := roleMachine(ts, rep).FireCtx(ctx, trackingdomain.RoleCurrent); err != nil {
		return nil, nil, err
	}
	now, err = ts.TrackingDomain().Get(ctx, rep.ID)
	return now, replaced, err
}

// Verify checks the Workspace's Tracking domain now, as the recheck job does, and returns it as
// the check left it (a retired host whose CNAME is gone is ErrNotFound afterwards).
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, id int64) (*ent.TrackingDomain, error) {
	if m.check == nil {
		return nil, ErrOff
	}
	row, err := s.TrackingDomain().Get(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	if err := m.checkRow(ctx, s, row); err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Recheck is the job's check of one Tracking domain by id. A row gone meanwhile is no error.
func (m *Module) Recheck(ctx context.Context, id int64) error {
	if m.check == nil {
		return nil
	}
	row, err := m.client.TrackingDomain.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return m.checkRow(ctx, m.client.Scoped(row.WorkspaceID), row)
}

// Due returns every Tracking domain whose host is verified, retired ones included, so a host that
// stops answering is demoted, and a retired one whose CNAME left is dropped, within one tick. A
// pending host is checked only when its Owner asks.
func (m *Module) Due(ctx context.Context) ([]int64, error) {
	return m.client.TrackingDomain.Query().Where(trackinghostlog.Serving()).IDs(ctx)
}

// checkRow re-checks one host and stores the outcome. A retired host is checked by DNS alone:
// gone, it is deleted and the Owners are told. A current or replacement host moves through the
// states (ADR 0038); a replacement that reaches serving takes over. A stale answer (the row moved
// meanwhile) and a host another Workspace holds are not errors: retrying cannot change them.
func (m *Module) checkRow(ctx context.Context, s *ent.Scoped, row *ent.TrackingDomain) error {
	if row.Role == trackingdomain.RoleRetired {
		return m.checkRetired(ctx, s, row)
	}
	next, err := m.check.next(ctx, row)
	if err != nil {
		return err
	}
	now := m.now()
	var promoted, replaced *ent.TrackingDomain
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		if next == row.State {
			return ts.TrackingDomain().UpdateOneID(row.ID).SetCheckedAt(now).ClearReason().Exec(ctx)
		}
		if err := stateMachine(ts, row, now).FireCtx(ctx, next); err != nil {
			return err
		}
		if err := pub.Publish(ctx, &events.TrackingDomainChanged{
			WorkspaceID: row.WorkspaceID, TrackingDomainID: row.ID, Host: row.Host,
			From: string(row.State), To: string(next),
		}); err != nil {
			return err
		}
		if row.Role == trackingdomain.RoleReplacement && next == trackingdomain.StateServing {
			var err error
			promoted, replaced, err = m.promote(ctx, ts, row)
			return err
		}
		return nil
	})
	switch {
	case db.IsUniqueViolation(err):
		// Another Workspace verified the host first. Record the check and why, keep the state.
		slog.InfoContext(ctx, "tracking host held by another workspace",
			"tracking_domain_id", row.ID, "workspace_id", row.WorkspaceID, "host", row.Host)
		return s.TrackingDomain().UpdateOneID(row.ID).
			SetCheckedAt(now).SetReason(trackingdomain.ReasonHostTaken).Exec(ctx)
	case errors.Is(err, lifecycle.ErrInvalidTransition), ent.IsNotFound(err):
		// Another check or a person moved the row after this one read it: the answer is stale.
		slog.InfoContext(ctx, "tracking domain changed during the check",
			"tracking_domain_id", row.ID, "workspace_id", row.WorkspaceID, "host", row.Host)
		return nil
	case err != nil:
		return err
	}
	if promoted != nil {
		m.notifyReplaced(ctx, promoted, replaced)
	} else if row.Role == trackingdomain.RoleCurrent && stateRank(next) < stateRank(row.State) {
		m.notify.TellBestEffort(ctx, row.WorkspaceID,
			ownernotice.Notice{Subject: i18n.EmailTrackingDomainRegressedSubject, Body: i18n.EmailTrackingDomainRegressedBody},
			map[string]any{"Host": row.Host, "From": row.State, "To": next, "Target": m.check.Target},
			"tracking_domain_id", row.ID)
	}
	return nil
}

// checkRetired keeps a retired host while its CNAME reaches the edge and deletes it once it does
// not, telling the Owners. A failed probe does not end it: DNS does (ADR 0071).
func (m *Module) checkRetired(ctx context.Context, s *ent.Scoped, row *ent.TrackingDomain) error {
	ok, err := m.check.pointsAtTarget(ctx, row.Host)
	if err != nil {
		return err
	}
	if ok {
		return notFoundIsStale(s.TrackingDomain().UpdateOneID(row.ID).
			Where(trackingdomain.RoleEQ(trackingdomain.RoleRetired)).SetCheckedAt(m.now()).Exec(ctx))
	}
	var gone int
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		var derr error
		gone, derr = ts.TrackingDomain().Delete().
			Where(trackingdomain.ID(row.ID), trackingdomain.RoleEQ(trackingdomain.RoleRetired)).Exec(ctx)
		return derr
	})
	if err != nil || gone == 0 {
		return err
	}
	m.notify.TellBestEffort(ctx, row.WorkspaceID,
		ownernotice.Notice{Subject: i18n.EmailTrackingDomainRetiredGoneSubject, Body: i18n.EmailTrackingDomainRetiredGoneBody},
		map[string]any{"Host": row.Host, "Target": m.check.Target}, "tracking_domain_id", row.ID)
	return nil
}

// notFoundIsStale drops ent's not-found of a conditional write: the row moved meanwhile.
func notFoundIsStale(err error) error {
	if ent.IsNotFound(err) {
		return nil
	}
	return err
}

func (m *Module) notifyReplaced(ctx context.Context, now, old *ent.TrackingDomain) {
	if old == nil {
		return
	}
	m.notify.TellBestEffort(ctx, now.WorkspaceID,
		ownernotice.Notice{Subject: i18n.EmailTrackingDomainReplacedSubject, Body: i18n.EmailTrackingDomainReplacedBody},
		map[string]any{"Host": now.Host, "Old": old.Host, "Target": m.check.Target}, "tracking_domain_id", now.ID)
}

// next is the state live DNS and the probe give a host: pending without the CNAME at Target,
// dns_verified with it, serving once the probe passes. Without an HTTP client nothing is promoted
// past what the row has. A resolver failure other than not-found is returned.
func (c *Checker) next(ctx context.Context, row *ent.TrackingDomain) (trackingdomain.State, error) {
	ok, err := c.pointsAtTarget(ctx, row.Host)
	if err != nil {
		return "", err
	}
	switch {
	case !ok:
		return trackingdomain.StatePending, nil
	case c.HTTP == nil:
		if row.State != trackingdomain.StatePending {
			return row.State, nil
		}
		return trackingdomain.StateDNSVerified, nil
	case c.probe(ctx, row.Host):
		return trackingdomain.StateServing, nil
	default:
		return trackingdomain.StateDNSVerified, nil
	}
}

// probe reports whether https://host/ProbePath answers 200 with the host's ProbeValue.
// Any failure, an error, another status or another body, is a failed probe.
func (c *Checker) probe(ctx context.Context, host string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+ProbePath, http.NoBody)
	if err != nil {
		return false
	}
	resp, err := c.HTTP.Do(req)
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
	return strings.TrimSpace(string(body)) == ProbeValue(c.Secret, host)
}

// pointsAtTarget reports whether host's CNAME resolves to Target. A missing record is
// false with a nil error; so is an answer of the host itself (no CNAME).
func (c *Checker) pointsAtTarget(ctx context.Context, host string) (bool, error) {
	cname, err := c.Lookup(ctx, host)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return normalize(cname) == normalize(c.Target) && normalize(cname) != normalize(host), nil
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
// host under secret (the Instance JWT secret), so it is tied to the host and cannot
// be guessed or copied from another host. Both sides compute it; nothing is stored.
func ProbeValue(secret, host string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("tracking-probe:" + normalize(host)))
	return hex.EncodeToString(mac.Sum(nil))
}

func byRole(rows []*ent.TrackingDomain, role trackingdomain.Role) *ent.TrackingDomain {
	for _, r := range rows {
		if r.Role == role {
			return r
		}
	}
	return nil
}

func byHost(rows []*ent.TrackingDomain, host string) *ent.TrackingDomain {
	for _, r := range rows {
		if r.Host == host {
			return r
		}
	}
	return nil
}

// stateRank orders the states from least to most verified.
func stateRank(s trackingdomain.State) int {
	switch s {
	case trackingdomain.StateServing:
		return 2
	case trackingdomain.StateDNSVerified:
		return 1
	default:
		return 0
	}
}

// stateMachine is the state's lifecycle (ADR 0046): every state can move to each of the others.
// A trigger is the state it moves to. The write is a compare-and-swap from the state the check
// read, and stamps served_at the first time the host serves.
func stateMachine(ts *ent.Scoped, row *ent.TrackingDomain, now time.Time) *stateless.StateMachine {
	m := lifecycle.New(lifecycle.Storage[trackingdomain.State]{
		Read: func(context.Context) (trackingdomain.State, error) { return row.State, nil },
		Write: func(ctx context.Context, from, to trackingdomain.State) (bool, error) {
			upd := ts.TrackingDomain().Update().
				Where(trackingdomain.ID(row.ID), trackingdomain.StateEQ(from), trackingdomain.RoleEQ(row.Role)).
				SetCheckedAt(now).ClearReason().SetState(to).SetStateChangedAt(now)
			if to == trackingdomain.StateServing && row.ServedAt == nil {
				upd.SetServedAt(now)
			}
			n, err := upd.Save(ctx)
			return n == 1, err
		},
	})
	states := []trackingdomain.State{trackingdomain.StatePending, trackingdomain.StateDNSVerified, trackingdomain.StateServing}
	for _, from := range states {
		cfg := m.Configure(from)
		for _, to := range states {
			if to != from {
				cfg.Permit(to, to)
			}
		}
	}
	return m
}

// roleMachine is the role's lifecycle (ADR 0046, ADR 0071). A trigger is the role it moves to: a
// current host is retired, a replacement becomes current, and a retired host is brought back as
// current or replacement. Deleting a row is not a move.
func roleMachine(ts *ent.Scoped, row *ent.TrackingDomain) *stateless.StateMachine {
	m := lifecycle.New(lifecycle.Storage[trackingdomain.Role]{
		Read: func(context.Context) (trackingdomain.Role, error) { return row.Role, nil },
		Write: func(ctx context.Context, from, to trackingdomain.Role) (bool, error) {
			n, err := ts.TrackingDomain().Update().
				Where(trackingdomain.ID(row.ID), trackingdomain.RoleEQ(from)).
				SetRole(to).Save(ctx)
			return n == 1, err
		},
	})
	m.Configure(trackingdomain.RoleCurrent).Permit(trackingdomain.RoleRetired, trackingdomain.RoleRetired)
	m.Configure(trackingdomain.RoleReplacement).Permit(trackingdomain.RoleCurrent, trackingdomain.RoleCurrent)
	m.Configure(trackingdomain.RoleRetired).
		Permit(trackingdomain.RoleCurrent, trackingdomain.RoleCurrent).
		Permit(trackingdomain.RoleReplacement, trackingdomain.RoleReplacement)
	return m
}

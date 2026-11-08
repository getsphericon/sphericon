// Package auditentries owns the Audit entries of the Enterprise Audit log (ADR 0022): the consumer that persists
// `audit.entry` domain events into the append-only audit_entries table, and the read
// side behind the /site page. It runs only under a license that includes the audit
// feature; without one nothing is stored and nothing is readable.
//
// Governed by ee/LICENSE, not the AGPL.
package auditentries

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// The unique constraint of the Audit entries table (ADR 0106): one entry per audit Event.
var _ = db.Declare(
	db.Fault("audit_entries_workspace_id_entry_key", "an entry is inserted ON CONFLICT (workspace_id, entry_key), the Event's envelope id"),
)

// Record stores the Audit entry an event carries, once: deduplicated by the envelope id, so a
// redelivered event writes one row. It is a raw-client entry (an envelope carries only a Workspace
// id, ADR 0017), run by the Audit log's consumer of the domain events (ADR 0101). Unlicensed, it
// consumes and discards.
func Record(ctx context.Context, client *ent.Client, lic *licensekey.License, env events.Envelope) error {
	if !lic.Has(licensekey.FeatureAudit) {
		return nil
	}
	ev, err := events.Decode(env)
	if err != nil {
		return err
	}
	entry, ok := ev.(*events.AuditEntry)
	if !ok {
		return nil
	}
	return persist(ctx, client, env, entry)
}

// Forwarder wraps the webhook dispatcher so `audit.entry` reaches Webhook endpoints
// only under a license that includes the audit feature (ADR 0022); without one nothing
// is delivered, whatever an endpoint's free-form event types list. Customer events pass
// through untouched.
func Forwarder(next events.WebhookDispatcher, lic *licensekey.License) events.WebhookDispatcher {
	return forwarder{next: next, lic: lic}
}

type forwarder struct {
	next events.WebhookDispatcher
	lic  *licensekey.License
}

func (f forwarder) Dispatch(ctx context.Context, s *ent.Scoped, eventName, deliveryID string, body []byte) error {
	if eventName == events.NameAuditEntry && !f.lic.Has(licensekey.FeatureAudit) {
		return nil
	}
	return f.next.Dispatch(ctx, s, eventName, deliveryID, body)
}

func persist(ctx context.Context, client *ent.Client, env events.Envelope, e *events.AuditEntry) error {
	create := client.Scoped(env.WorkspaceID).AuditEntry().Create().
		SetEntryKey(env.ID).
		SetOccurredAt(env.OccurredAt).
		SetActorKind(e.Actor.Kind).
		SetAction(e.Action).
		SetTargetType(e.TargetType)
	if e.Actor.ID != "" {
		create.SetActorID(e.Actor.ID)
	}
	if e.Actor.Name != "" {
		create.SetActorName(e.Actor.Name)
	}
	if e.TargetID != "" {
		create.SetTargetID(e.TargetID)
	}
	if e.TargetName != "" {
		create.SetTargetName(e.TargetName)
	}
	if len(e.Diff) > 0 {
		create.SetDiff(e.Diff)
	}
	if e.RequestID != "" {
		create.SetRequestID(e.RequestID)
	}
	if e.IP != "" {
		create.SetIP(e.IP)
	}
	if e.UserAgent != "" {
		create.SetUserAgent(e.UserAgent)
	}
	if err := create.
		OnConflictColumns(auditentry.FieldWorkspaceID, auditentry.FieldEntryKey).
		Ignore().
		Exec(ctx); err != nil {
		return fmt.Errorf("persist audit entry %q: %w", env.ID, err)
	}
	return nil
}

// PruneBefore deletes the scoped Workspace's Audit entries that occurred before cutoff and returns
// how many it removed: the advanced-retention control's prune (ee/usecase/auditlogretention), the one remover of
// entries besides the erasure of a deleted Workspace.
func PruneBefore(ctx context.Context, s *ent.Scoped, cutoff time.Time) (int, error) {
	return s.AuditEntry().Delete().Where(auditentry.OccurredAtLT(cutoff)).Exec(ctx)
}

// Log is the read side of the Audit log, and the export, which records itself through the bus.
// Its zero value is unlicensed.
type Log struct {
	lic *licensekey.License
	bus *events.Bus
}

// NewLog builds the reader under a license, recording an export through bus.
func NewLog(lic *licensekey.License, bus *events.Bus) *Log { return &Log{lic: lic, bus: bus} }

// Licensed reports whether this instance may show an Audit log.
func (l *Log) Licensed() bool { return l != nil && l.lic.Has(licensekey.FeatureAudit) }

// RetentionLicensed reports whether this instance may set an Audit log retention window.
func (l *Log) RetentionLicensed() bool { return l != nil && l.lic.Has(licensekey.FeatureRetention) }

// Entries is a page of the scoped Workspace's entries matching the filter, newest first: up to
// limit entries that precede the cursor (an entry id; 0 starts from the newest), and the cursor
// of the next page (0 on the last page). The caller has already checked Licensed and the
// reader's role.
func (l *Log) Entries(ctx context.Context, s *ent.Scoped, f events.AuditFilter, cursor int64, limit int) (pagination.IDPage[*ent.AuditEntry], error) {
	q := s.AuditEntry().Query().Where(predicates(f)...).Order(ent.Desc(auditentry.FieldID)).Limit(limit + 1)
	if cursor > 0 {
		q = q.Where(auditentry.IDLT(cursor))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return pagination.IDPage[*ent.AuditEntry]{}, err
	}
	return pagination.ByID(rows, limit, func(e *ent.AuditEntry) int64 { return e.ID }), nil
}

// Facets returns the values that occur in the scoped Workspace's Audit log, sorted, for its
// filter. Each actor carries the name of its newest entry. A staff identity is never exposed
// (ADR 0022): staff entries collapse into one id-less staff actor. The caller has already
// checked Licensed and the reader's role.
func (l *Log) Facets(ctx context.Context, s *ent.Scoped) (auditlog.Facets, error) {
	var rows []struct {
		Kind   string  `json:"actor_kind"`
		ID     *string `json:"actor_id"`
		Name   *string `json:"actor_name"`
		Newest int64   `json:"newest"`
	}
	err := s.AuditEntry().Query().
		Where(auditentry.ActorKindNotIn(events.ActorStaff, events.ActorService)).
		GroupBy(auditentry.FieldActorKind, auditentry.FieldActorID, auditentry.FieldActorName).
		Aggregate(ent.As(ent.Max(auditentry.FieldID), "newest")).
		Scan(ctx, &rows)
	if err != nil {
		return auditlog.Facets{}, fmt.Errorf("audit facets actors: %w", err)
	}
	staff, err := s.AuditEntry().Query().
		Where(auditentry.ActorKindIn(events.ActorStaff, events.ActorService)).
		Exist(ctx)
	if err != nil {
		return auditlog.Facets{}, fmt.Errorf("audit facets staff: %w", err)
	}
	actions, err := s.AuditEntry().Query().Unique(true).Select(auditentry.FieldAction).Strings(ctx)
	if err != nil {
		return auditlog.Facets{}, fmt.Errorf("audit facets actions: %w", err)
	}
	targetTypes, err := s.AuditEntry().Query().Unique(true).Select(auditentry.FieldTargetType).Strings(ctx)
	if err != nil {
		return auditlog.Facets{}, fmt.Errorf("audit facets target types: %w", err)
	}

	// One row per name an actor has had: keep the name of its newest entry.
	type key struct{ kind, id string }
	newest := map[key]int64{}
	byKey := map[key]auditlog.Actor{}
	for _, r := range rows {
		k := key{r.Kind, lo.FromPtr(r.ID)}
		if r.Newest > newest[k] {
			newest[k] = r.Newest
			byKey[k] = auditlog.Actor{Kind: r.Kind, ID: r.ID, Name: r.Name}
		}
	}
	actors := make([]auditlog.Actor, 0, len(byKey)+1)
	for _, a := range byKey {
		actors = append(actors, a)
	}
	if staff {
		actors = append(actors, auditlog.StaffActor())
	}
	slices.SortFunc(actors, func(a, b auditlog.Actor) int {
		return cmp.Or(
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(lo.FromPtr(a.Name), lo.FromPtr(b.Name)),
			cmp.Compare(lo.FromPtr(a.ID), lo.FromPtr(b.ID)),
		)
	})
	slices.Sort(actions)
	slices.Sort(targetTypes)
	return auditlog.Facets{Actors: actors, Actions: actions, TargetTypes: targetTypes}, nil
}

// predicates turns the filter into query predicates; an unset field adds none.
func predicates(f events.AuditFilter) []predicate.AuditEntry {
	var ps []predicate.AuditEntry
	if !f.From.IsZero() {
		ps = append(ps, auditentry.OccurredAtGTE(f.From))
	}
	if !f.To.IsZero() {
		ps = append(ps, auditentry.OccurredAtLT(f.To))
	}
	// The staff member kind covers every staff identity (a Service credential reads as an
	// Staff member to a customer), so the filter matches both.
	if f.ActorKind == events.ActorStaff {
		ps = append(ps, auditentry.ActorKindIn(events.ActorStaff, events.ActorService))
	} else if f.ActorKind != "" {
		ps = append(ps, auditentry.ActorKind(f.ActorKind))
	}
	// A staff id is never exposed, so it must not be probeable either: with the
	// staff member kind the id filter is ignored, and without a kind an id filter never
	// matches a staff entry (ADR 0022).
	if f.ActorID != "" && f.ActorKind != events.ActorStaff {
		ps = append(ps, auditentry.ActorID(f.ActorID), auditentry.ActorKindNotIn(events.ActorStaff, events.ActorService))
	}
	if f.Action != "" {
		ps = append(ps, auditentry.Action(f.Action))
	}
	if f.TargetType != "" {
		ps = append(ps, auditentry.TargetType(f.TargetType))
	}
	if f.TargetID != "" {
		ps = append(ps, auditentry.TargetID(f.TargetID))
	}
	if f.IP != "" {
		ps = append(ps, auditentry.IP(f.IP))
	}
	if f.RequestID != "" {
		ps = append(ps, auditentry.RequestID(f.RequestID))
	}
	return ps
}

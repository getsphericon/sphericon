// Package audit is the Enterprise Audit log (ADR 0022): the subscriber that persists
// `audit.entry` domain events into the append-only audit_entries table, and the read
// side behind the /site page. It runs only under a license that includes the audit
// feature; without one nothing is stored and nothing is readable.
//
// Governed by ee/LICENSE, not the AGPL.
package audit

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/events"
)

// ConsumerName names the bus consumer group of the audit subscriber.
const ConsumerName = events.GroupAudit

// Consumer is the audit subscriber. It is the raw-client entry for audit entries (an
// event envelope carries only a Workspace id, ADR 0017). It persists `audit.entry`
// events, deduplicated by the envelope id so at-least-once redelivery writes one row,
// and ignores every other event. Unlicensed, it consumes and discards.
func Consumer(client *ent.Client, lic *licensekey.License) events.Consumer {
	return events.Consumer{
		Name: ConsumerName,
		Handle: func(ctx context.Context, env events.Envelope) error {
			if env.Name != events.NameAuditEntry || !lic.Has(licensekey.FeatureAudit) {
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
		},
	}
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

// Log is the read side of the Audit log. Its zero value is unlicensed.
type Log struct {
	lic *licensekey.License
}

// NewLog builds the reader under a license.
func NewLog(lic *licensekey.License) *Log { return &Log{lic: lic} }

// Licensed reports whether this instance may show an Audit log.
func (l *Log) Licensed() bool { return l != nil && l.lic.Has(licensekey.FeatureAudit) }

// RetentionLicensed reports whether this instance may set an Audit log retention window.
func (l *Log) RetentionLicensed() bool { return l != nil && l.lic.Has(licensekey.FeatureRetention) }

// Entries returns up to limit entries of the scoped Workspace matching the filter,
// newest first, that precede the cursor (an entry id; 0 starts from the newest), and the cursor of the
// next page (0 on the last page). The caller has already checked Licensed and the
// reader's role.
func (l *Log) Entries(ctx context.Context, s *ent.Scoped, f events.AuditFilter, cursor int64, limit int) ([]*ent.AuditEntry, int64, error) {
	q := s.AuditEntry().Query().Where(predicates(f)...).Order(ent.Desc(auditentry.FieldID)).Limit(limit + 1)
	if cursor > 0 {
		q = q.Where(auditentry.IDLT(cursor))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) <= limit {
		return rows, 0, nil
	}
	rows = rows[:limit]
	return rows, rows[limit-1].ID, nil
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
	if f.ActorKind != "" {
		ps = append(ps, auditentry.ActorKind(f.ActorKind))
	}
	// An Operator's id is never exposed, so it must not be probeable either: with the
	// operator kind the id filter is ignored, and without a kind an id filter never
	// matches an Operator's entries (ADR 0022).
	if f.ActorID != "" && f.ActorKind != events.ActorOperator {
		ps = append(ps, auditentry.ActorID(f.ActorID), auditentry.ActorKindNEQ(events.ActorOperator))
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

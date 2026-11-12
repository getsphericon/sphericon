// Package auditlog serves the Audit log (GLOSSARY "Audit entry", ADR 0022) to every surface that
// reads it: a page of a Workspace's entries, the choices of its filter, its export and its
// retention window, each refused as unlicensed on an Instance whose License lacks the feature
// (Log), and the cursor handling and the customer-facing view of an entry, so the surfaces map one
// result and cannot diverge on masking (ADR 0111). The Staff console's view, which names a Service
// credential, is its own module's (ee/domain/insights/workspaceoversight).
package auditlog

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/go-faster/jx"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrBadCursor is returned for a cursor that is not a positive entry id.
var ErrBadCursor = validation.UnknownReference("auditapi: invalid cursor", i18n.ErrorsAuditCursorInvalid)

// Cursor parses the optional cursor (empty starts from the newest entry).
func Cursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	after, err := publicid.Decode(cursor)
	if err != nil || after < 1 {
		return 0, ErrBadCursor
	}
	return after, nil
}

// NextCursor renders the cursor of the next page.
func NextCursor(next int64) string { return publicid.Encode(next) }

// View is an Audit entry as a customer sees it. A Staff member appears only as
// events.StaffLabel, with no id.
type View struct {
	ID         int64
	OccurredAt time.Time
	ActorKind  string
	ActorID    *string
	ActorName  *string
	Action     string
	TargetType string
	TargetID   *string
	TargetName *string
	Diff       map[string]jx.Raw
	RequestID  *string
	IP         *string
	UserAgent  *string
}

// ViewOf builds the customer-facing view of a stored entry: every platform staff identity, a
// Service credential among them, as events.StaffLabel with no id.
func ViewOf(e *ent.AuditEntry) View {
	v := View{
		ID:         e.ID,
		OccurredAt: e.OccurredAt,
		ActorKind:  string(e.ActorKind),
		ActorID:    e.ActorID,
		ActorName:  e.ActorName,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		TargetName: e.TargetName,
		RequestID:  e.RequestID,
		IP:         e.IP,
		UserAgent:  e.UserAgent,
	}
	if events.IsStaff(string(e.ActorKind)) {
		label := events.StaffLabel
		v.ActorKind, v.ActorID, v.ActorName = events.ActorStaff, nil, &label
	}
	if len(e.Diff) > 0 {
		v.Diff = make(map[string]jx.Raw, len(e.Diff))
		for k, val := range e.Diff {
			if unchanged(val) {
				continue
			}
			if raw, err := json.Marshal(val); err == nil {
				v.Diff[k] = jx.Raw(raw)
			}
		}
	}
	return v
}

// Page is one page of a Workspace's Audit log, newest first: its entries as the reader is shown
// them and the cursor of the next page ("" on the last).
type Page struct {
	Entries []View
	Next    string
}

// pageOf is the customer-facing Page of a read.
func pageOf(p pagination.IDPage[*ent.AuditEntry]) Page {
	out := Page{Entries: make([]View, len(p.Items))}
	for i, e := range p.Items {
		out.Entries[i] = ViewOf(e)
	}
	if p.Next > 0 {
		out.Next = NextCursor(p.Next)
	}
	return out
}

// unchanged reports a diff entry recorded only to classify the entry (schema.Audited
// Always): its before and after are equal, so a reader is not shown it as a change.
func unchanged(v any) bool {
	change, ok := v.(map[string]any)
	if !ok {
		return false
	}
	from, hasFrom := change[events.DiffFrom]
	to, hasTo := change[events.DiffTo]
	return hasFrom && hasTo && reflect.DeepEqual(from, to)
}

// Actor is an actor of the Audit log as a customer sees it: a Staff member appears only as
// events.StaffLabel, with no id.
type Actor struct {
	Kind string
	ID   *string
	Name *string
}

// Facets are the values that occur in a Workspace's Audit log, offered as the choices of its
// filter: every actor, with its latest name snapshot, every action and every target type.
type Facets struct {
	Actors      []Actor
	Actions     []string
	TargetTypes []string
}

// StaffActor is the one id-less actor that stands for every platform staff identity.
func StaffActor() Actor {
	label := events.StaffLabel
	return Actor{Kind: events.ActorStaff, Name: &label}
}

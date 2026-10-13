// Package auditapi holds what the /site and /api read surfaces of the Audit log share
// (ADR 0022): cursor and page-size handling, and the customer-facing view of an entry,
// so the two handler packages map one shape and cannot diverge on masking.
package auditapi

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"time"

	"github.com/go-faster/jx"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/holds"
)

const (
	defaultLimit = 25
	maxLimit     = 100
)

// ErrBadCursor is returned for a cursor that is not a positive entry id.
var ErrBadCursor = errors.New("invalid cursor")

// Page parses the optional cursor (empty starts from the newest entry) and clamps
// the page size: an absent or out-of-range limit becomes the default.
func Page(cursor string, limit int32, limitSet bool) (int64, int, error) {
	var after int64
	if cursor != "" {
		var err error
		if after, err = strconv.ParseInt(cursor, 10, 64); err != nil || after < 1 {
			return 0, 0, ErrBadCursor
		}
	}
	n := defaultLimit
	if limitSet {
		n = int(limit)
	}
	if n < 1 || n > maxLimit {
		n = defaultLimit
	}
	return after, n, nil
}

// NextCursor renders the cursor of the next page.
func NextCursor(next int64) string { return strconv.FormatInt(next, 10) }

// View is an Audit entry as a customer sees it. An Operator appears only as
// events.OperatorLabel, with no id.
type View struct {
	ID         string
	OccurredAt time.Time
	ActorKind  string
	ActorID    *string
	ActorName  *string
	Action     string
	// Billing is true for a change of a billing hold (holds.IsBilling): its set, renewal
	// and lift alike, since a hold's audit diff always carries its reason code.
	Billing    bool
	TargetType string
	TargetID   *string
	TargetName *string
	Diff       map[string]jx.Raw
	RequestID  *string
	IP         *string
	UserAgent  *string
}

// ViewOf builds the customer-facing view of a stored entry.
func ViewOf(e *ent.AuditEntry) View {
	v := View{
		ID:         strconv.FormatInt(e.ID, 10),
		OccurredAt: e.OccurredAt,
		ActorKind:  e.ActorKind,
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
	if events.IsStaff(e.ActorKind) {
		label := events.OperatorLabel
		v.ActorKind, v.ActorID, v.ActorName = events.ActorOperator, nil, &label
	}
	v.Billing = isBillingHoldChange(e)
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

// unchanged reports a diff entry recorded only to classify the entry (schema.Audited
// Always): its before and after are equal, so a reader is not shown it as a change.
func unchanged(v any) bool {
	change, ok := v.(map[string]any)
	if !ok {
		return false
	}
	from, hasFrom := change["from"]
	to, hasTo := change["to"]
	return hasFrom && hasTo && reflect.DeepEqual(from, to)
}

// isBillingHoldChange reads the reason code a hold change set, renewed or removed from
// its diff: a create and a delete carry every field and an update always carries the
// reason code (schema.Audited Always on Hold), changed or not.
func isBillingHoldChange(e *ent.AuditEntry) bool {
	if e.TargetType != "hold" {
		return false
	}
	change, ok := e.Diff["reason_code"].(map[string]any)
	if !ok {
		return false
	}
	for _, side := range []string{"from", "to"} {
		if code, ok := change[side].(string); ok && holds.IsBilling(code) {
			return true
		}
	}
	return false
}

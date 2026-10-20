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

// ViewOf builds the customer-facing view of a stored entry.
func ViewOf(e *ent.AuditEntry) View {
	v := View{
		ID:         e.ID,
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

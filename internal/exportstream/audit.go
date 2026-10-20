package exportstream

import (
	"context"
	"encoding/json"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/auditapi"
)

// AuditSchema is the TypeSpec row schema (openapi component) an AuditRow encodes to.
const AuditSchema = "ExportAuditRow"

// AuditRow is one line of the Audit stream, the contract's ExportAuditRow: every key is
// always present, an absent value is null, ids are decimal strings like everywhere in /api.
// Entries are append-only, so a row is delivered once.
type AuditRow struct {
	ID         int64                      `json:"id,string"`
	OccurredAt time.Time                  `json:"occurred_at"`
	ActorKind  string                     `json:"actor_kind"`
	ActorID    *string                    `json:"actor_id"`
	ActorName  *string                    `json:"actor_name"`
	Action     string                     `json:"action"`
	TargetType string                     `json:"target_type"`
	TargetID   *string                    `json:"target_id"`
	TargetName *string                    `json:"target_name"`
	Diff       map[string]json.RawMessage `json:"diff"`
	RequestID  *string                    `json:"request_id"`
	IP         *string                    `json:"ip"`
	UserAgent  *string                    `json:"user_agent"`
	CreatedAt  time.Time                  `json:"created_at"`
	UpdatedAt  time.Time                  `json:"updated_at"`
}

// Position is the row's (updated_at, id).
func (r AuditRow) Position() Cursor { return Cursor{At: r.UpdatedAt, ID: r.ID} }

// auditRow reads an entry through the customer-facing view, so a Staff member is
// "Sphericon staff" with no id and unchanged "Always" fields are left out, as on every
// other read surface (ADR 0022).
func auditRow(e *ent.AuditEntry) AuditRow {
	v := auditapi.ViewOf(e)
	diff := make(map[string]json.RawMessage, len(v.Diff))
	for k, raw := range v.Diff {
		diff[k] = json.RawMessage(raw)
	}
	return AuditRow{
		ID:         v.ID,
		OccurredAt: v.OccurredAt.UTC(),
		ActorKind:  v.ActorKind,
		ActorID:    v.ActorID,
		ActorName:  v.ActorName,
		Action:     v.Action,
		TargetType: v.TargetType,
		TargetID:   v.TargetID,
		TargetName: v.TargetName,
		Diff:       diff,
		RequestID:  v.RequestID,
		IP:         v.IP,
		UserAgent:  v.UserAgent,
		CreatedAt:  e.CreatedAt.UTC(),
		UpdatedAt:  e.UpdatedAt.UTC(),
	}
}

// Audit serves a page of the Workspace's Audit stream. The caller has checked the `audit`
// license, which is an Enterprise feature of the entries themselves.
func Audit(ctx context.Context, s *ent.Scoped, token string, limit int) (Page, error) {
	return Read(token, limit, func(after Cursor, n int) ([]AuditRow, error) {
		rows, err := s.AuditEntry().Query().
			Where(predicate.AuditEntry(Seek(after))).
			Order(Order).
			Limit(n).
			All(ctx)
		if err != nil {
			return nil, err
		}
		return lo.Map(rows, func(e *ent.AuditEntry, _ int) AuditRow { return auditRow(e) }), nil
	})
}

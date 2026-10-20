package exportstream

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// ErasuresSchema is the TypeSpec row schema (openapi component) an ErasureRow encodes to.
const ErasuresSchema = "ExportErasureRow"

// ErasureRow is one line of the Erasures stream, the contract's ExportErasureRow: the keys an
// Erasure matched on. Every key is always present, an absent value is null, ids are decimal
// strings like everywhere in /api. The hashes are SHA-256 as lowercase hex; the address and the
// phone themselves are never stored, so they cannot be here.
type ErasureRow struct {
	ID          int64     `json:"id,string"`
	ContactID   *int64    `json:"contact_id,string"`
	SubjectID   string    `json:"subject_id"`
	VisitorIDs  []string  `json:"visitor_ids"`
	EmailSHA256 *string   `json:"email_sha256"`
	PhoneSHA256 *string   `json:"phone_sha256"`
	ErasedAt    time.Time `json:"erased_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Position is the row's (updated_at, id).
func (r ErasureRow) Position() Cursor { return Cursor{At: r.UpdatedAt, ID: r.ID} }

func erasureRow(e *ent.Erasure) ErasureRow {
	return ErasureRow{
		ID:          e.ID,
		ContactID:   e.ContactID,
		SubjectID:   e.SubjectID,
		VisitorIDs:  lo.Ternary(e.VisitorIds == nil, []string{}, e.VisitorIds),
		EmailSHA256: e.EmailSha256,
		PhoneSHA256: e.PhoneSha256,
		ErasedAt:    e.ErasedAt.UTC(),
		CreatedAt:   e.CreatedAt.UTC(),
		UpdatedAt:   e.UpdatedAt.UTC(),
	}
}

// Erasures serves a page of the Workspace's Erasures stream.
func Erasures(ctx context.Context, s *ent.Scoped, token string, limit int) (Page, error) {
	return Read(token, limit, func(after Cursor, n int) ([]ErasureRow, error) {
		rows, err := s.Erasure().Query().
			Where(predicate.Erasure(Seek(after))).
			Order(Order).
			Limit(n).
			All(ctx)
		if err != nil {
			return nil, err
		}
		return lo.Map(rows, func(e *ent.Erasure, _ int) ErasureRow { return erasureRow(e) }), nil
	})
}

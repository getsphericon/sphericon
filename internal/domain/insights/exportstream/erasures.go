package exportstream

import (
	"context"
	"fmt"
	"time"

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
	ID         int64     `json:"id,string"`
	ContactID  *int64    `json:"contact_id,string"`
	SubjectID  string    `json:"subject_id"`
	VisitorIDs []string  `json:"visitor_ids"`
	EmailHash  *string   `json:"email_hash"`
	PhoneHash  *string   `json:"phone_hash"`
	ErasedAt   time.Time `json:"erased_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Position is the row's (updated_at, id).
func (r ErasureRow) Position() Cursor { return Cursor{At: r.UpdatedAt, ID: r.ID} }

// erasureRow is the stream's row of an Erasure, its hashes opened with the cipher: a hash that does
// not open fails the page rather than leave the row out or hand out ciphertext.
func (st *Streams) erasureRow(e *ent.Erasure) (ErasureRow, error) {
	email, err := st.open(e.EmailHashEncrypted)
	if err != nil {
		return ErasureRow{}, err
	}
	phone, err := st.open(e.PhoneHashEncrypted)
	if err != nil {
		return ErasureRow{}, err
	}
	return ErasureRow{
		ID:         e.ID,
		ContactID:  e.ContactID,
		SubjectID:  e.SubjectID,
		VisitorIDs: e.VisitorIds,
		EmailHash:  email,
		PhoneHash:  phone,
		ErasedAt:   e.ErasedAt.UTC(),
		CreatedAt:  e.CreatedAt.UTC(),
		UpdatedAt:  e.UpdatedAt.UTC(),
	}, nil
}

// open is a sealed hash in the clear; nil for none.
func (st *Streams) open(sealed *string) (*string, error) {
	if sealed == nil {
		return nil, nil
	}
	hash, err := st.cipher.Decrypt(*sealed)
	if err != nil {
		return nil, fmt.Errorf("open an erasure hash: %w", err)
	}
	return new(string(hash)), nil
}

// Erasures serves a page of the Workspace's Erasures stream.
func (st *Streams) Erasures(ctx context.Context, s *ent.Scoped, token string, limit int) (Page, error) {
	if !st.offered() {
		return Page{}, ErrNotOffered
	}
	return Read(token, limit, func(after Cursor, n int) ([]ErasureRow, error) {
		rows, err := s.Erasure().Query().
			Where(predicate.Erasure(Seek(after))).
			Order(Order).
			Limit(n).
			All(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]ErasureRow, len(rows))
		for i, e := range rows {
			if out[i], err = st.erasureRow(e); err != nil {
				return nil, err
			}
		}
		return out, nil
	})
}

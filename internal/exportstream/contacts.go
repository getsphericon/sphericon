package exportstream

import (
	"context"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// ContactsSchema is the TypeSpec row schema (openapi component) a ContactRow encodes to.
const ContactsSchema = "ExportContactRow"

// ContactRow is one line of the Contacts stream, the contract's ExportContactRow: every
// key is always present, an absent value is null, Custom fields are one JSON column and
// the Tags are their names.
type ContactRow struct {
	ID           int64          `json:"id,string"`
	SubjectID    *string        `json:"subject_id"`
	Email        *string        `json:"email"`
	Phone        *string        `json:"phone"`
	FirstName    *string        `json:"first_name"`
	LastName     *string        `json:"last_name"`
	TimeZone     *string        `json:"time_zone"`
	Country      *string        `json:"country"`
	Locale       *string        `json:"locale"`
	Birthday     *string        `json:"birthday"`
	City         *string        `json:"city"`
	Avatar       *string        `json:"avatar"`
	CustomFields map[string]any `json:"custom_fields"`
	Tags         []string       `json:"tags"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// dateOnly is a calendar date as the contract writes it (`2006-01-02`), nil for none.
func dateOnly(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return new(t.Format(time.DateOnly))
}

// Position is the row's (updated_at, id).
func (r ContactRow) Position() Cursor { return Cursor{At: r.UpdatedAt, ID: r.ID} }

func contactRow(c *ent.Contact) ContactRow {
	names := lo.Map(c.Edges.Tags, func(t *ent.Tag, _ int) string { return t.Name })
	slices.Sort(names)
	return ContactRow{
		ID:           c.ID,
		SubjectID:    c.SubjectID,
		Email:        c.Email,
		Phone:        c.Phone,
		FirstName:    c.FirstName,
		LastName:     c.LastName,
		TimeZone:     c.TimeZone,
		Country:      c.Country,
		Locale:       c.Locale,
		Birthday:     dateOnly(c.Birthday),
		City:         c.City,
		Avatar:       c.Avatar,
		CustomFields: lo.Ternary(c.CustomFields == nil, map[string]any{}, c.CustomFields),
		Tags:         names,
		CreatedAt:    c.CreatedAt.UTC(),
		UpdatedAt:    c.UpdatedAt.UTC(),
	}
}

// Contacts serves a page of the Workspace's Contacts stream. A Tag change moves the
// Contact's updated_at (internal/tags), so the Tag names ride the row's own version.
func Contacts(ctx context.Context, s *ent.Scoped, token string, limit int) (Page, error) {
	return Read(token, limit, func(after Cursor, n int) ([]ContactRow, error) {
		rows, err := s.Contact().Query().
			Where(predicate.Contact(Seek(after))).
			Order(Order).
			Limit(n).
			WithTags().
			All(ctx)
		if err != nil {
			return nil, err
		}
		return lo.Map(rows, func(c *ent.Contact, _ int) ContactRow { return contactRow(c) }), nil
	})
}

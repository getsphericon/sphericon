package contacts

import (
	"context"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactdate"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
)

// The keys of a Contact's dates (ContactDate, ADR 0126): a row for each date it holds, its
// birthday and each Custom field value that is a date, by the key the date scheduler's band finds
// it by (datetrigger.Holder.Keys). Every writer of a Contact keeps them from the row it wrote, in
// its transaction, beside the send lock of its date fields.

// keysOf are the keys of the dates the Contact holds, none for a Contact not made yet.
func keysOf(c *ent.Contact) []datetrigger.Keyed {
	if c == nil {
		return nil
	}
	return datetrigger.Holder{Birthday: c.Birthday, CustomFields: c.CustomFields}.Keys()
}

// keepDates rewrites the keys of the Contact's dates once a write took it from before (nil for a
// Contact the write made) to after, in the transaction s is bound to: a write that leaves its
// dates as they were writes nothing.
func keepDates(ctx context.Context, s *ent.Scoped, before, after *ent.Contact) error {
	held, keys := keysOf(before), keysOf(after)
	if slices.Equal(held, keys) {
		return nil
	}
	if len(held) > 0 {
		if _, err := s.ContactDate().Delete().Where(contactdate.ContactID(after.ID)).Exec(ctx); err != nil {
			return err
		}
	}
	rows := make([]*ent.ContactDateScopedCreate, len(keys))
	for i, k := range keys {
		rows[i] = s.ContactDate().Create().
			SetContactID(after.ID).
			SetKind(k.Field.Kind).
			SetKey(k.Field.Key).
			SetDate(k.Key.Midnight()).
			SetMonthDay(k.Key.MonthDay())
	}
	return s.ContactDate().CreateBulk(rows...).Exec(ctx)
}

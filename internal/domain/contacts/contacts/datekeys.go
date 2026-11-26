package contacts

import (
	"context"
	"reflect"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactdate"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
)

// The dates a Contact holds (ContactDate, ADR 0126): a row for each, its birthday and each Custom
// field value that is a date, by the key the date scheduler's band finds it by
// (datetrigger.Holder.Keys), with when the write that set the key was made and whether it came
// through a Connection without mcp:send (ADR 0132 as amended by ADR 0126). Every writer of a
// Contact keeps them from the row it wrote, in its transaction (keepDates).

// The unique constraint of a Contact's dates (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault("contact_dates_field_contact",
		"a Contact's one row per date field is written by keepDates alone, which deletes a field's row before it writes another, in the transaction of the Contact's write"),
)

// DatesOf is what the Contact holds of the dates (datetrigger.Holder): its birthday and its Custom
// field values, nothing for a Contact not made yet. Every reader of a Contact's dates takes them
// from here.
func DatesOf(c *ent.Contact) datetrigger.Holder {
	if c == nil {
		return datetrigger.Holder{}
	}
	return datetrigger.Holder{Birthday: c.Birthday, CustomFields: c.CustomFields}
}

// keepDates keeps the dates of the Contact once a write in ctx took it from before (nil for a
// Contact the write made) to after, in the transaction s is bound to. A field whose stored value
// the write changed gets a new row, written now and send-locked when ctx carries the mark, even
// when its key stays the same (ADR 0132 as amended by ADR 0126: a write of the field without the
// mark lifts it); a field that no longer holds a date loses its row; a field whose value is as it
// was keeps its row, its write time and its send lock.
func (m *Module) keepDates(ctx context.Context, s *ent.Scoped, before, after *ent.Contact) error {
	was, is := DatesOf(before), DatesOf(after)
	held := keyMap(was.Keys())
	keys := is.Keys()
	same := func(f datetrigger.Field) bool { return sameValue(was, is, f) }
	fresh := lo.Filter(keys, func(k datetrigger.Keyed, _ int) bool {
		_, ok := held[k.Field]
		return !ok || !same(k.Field)
	})
	kept := keyMap(keys)
	gone := lo.Filter(lo.Keys(held), func(f datetrigger.Field, _ int) bool {
		_, ok := kept[f]
		return !ok || !same(f)
	})
	if len(gone) > 0 {
		fields := lo.Map(gone, func(f datetrigger.Field, _ int) predicate.ContactDate {
			return contactdate.And(contactdate.KindEQ(f.Kind), contactdate.Key(f.Key))
		})
		if _, err := s.ContactDate().Delete().Where(contactdate.ContactID(after.ID), contactdate.Or(fields...)).Exec(ctx); err != nil {
			return err
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	now, locked := m.now(), events.IsSendLocked(ctx)
	rows := lo.Map(fresh, func(k datetrigger.Keyed, _ int) *ent.ContactDateScopedCreate {
		return s.ContactDate().Create().
			SetContactID(after.ID).
			SetKind(k.Field.Kind).
			SetKey(k.Field.Key).
			SetDate(k.Key.Midnight()).
			SetMonthDay(k.Key.MonthDay()).
			SetWrittenAt(now).
			SetSendLocked(locked)
	})
	return s.ContactDate().CreateBulk(rows...).Exec(ctx)
}

// keyMap is the keys by their field.
func keyMap(keys []datetrigger.Keyed) map[datetrigger.Field]localcalendar.Date {
	return lo.SliceToMap(keys, func(k datetrigger.Keyed) (datetrigger.Field, localcalendar.Date) { return k.Field, k.Key })
}

// sameValue reports whether the two holdings store the same value in the field: the same birthday,
// or the same Custom field value as stored (storedValue).
func sameValue(was, is datetrigger.Holder, f datetrigger.Field) bool {
	switch f.Kind {
	case datetrigger.Birthday:
		return was.Birthday != nil && is.Birthday != nil && was.Birthday.Equal(*is.Birthday)
	case datetrigger.CustomField:
		return reflect.DeepEqual(storedValue(was.CustomFields[f.Key]), storedValue(is.CustomFields[f.Key]))
	}
	return false
}

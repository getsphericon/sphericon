package contacts

import (
	"cmp"
	"context"
	"reflect"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
)

// The send lock of a date field (ADR 0132 as amended by ADR 0126): a Contact keeps the date fields,
// its birthday and its datetime Custom fields, whose last write came through a Connection without
// mcp:send (events.IsSendLocked), and a date Trigger enrolls no one on an occurrence of such a
// value. A write that gives a field another value marks it when it carries the mark and takes the
// mark off otherwise; a write that leaves the value as it was changes no mark.

// dates is what a Contact holds of its date fields: its birthday and its Custom field values.
type dates struct {
	birthday *time.Time
	fields   map[string]any
}

// datesOf is what the Contact holds, nothing for a Contact not made yet.
func datesOf(c *ent.Contact) dates {
	if c == nil {
		return dates{}
	}
	return dates{birthday: c.Birthday, fields: c.CustomFields}
}

// sendLocks is the Contact's marks once a write in ctx takes it from before to after, and whether
// they differ from the marks it held (held, none for a Contact not made yet): each date field whose
// value the write changes is marked when ctx carries the mark and unmarked otherwise. Only the
// datetime Custom fields of the Workspace's catalogue are date fields.
func sendLocks(ctx context.Context, s *ent.Scoped, held []datetrigger.Field, before, after dates) ([]datetrigger.Field, bool, error) {
	var written []datetrigger.Field
	if !sameDay(before.birthday, after.birthday) {
		written = append(written, datetrigger.Field{Kind: datetrigger.Birthday})
	}
	changed := lo.Filter(lo.Uniq(append(lo.Keys(before.fields), lo.Keys(after.fields)...)), func(key string, _ int) bool {
		return !reflect.DeepEqual(storedValue(before.fields[key]), storedValue(after.fields[key]))
	})
	if len(changed) > 0 {
		var datetimes []string
		err := s.CustomField().Query().
			Where(customfield.KeyIn(changed...), customfield.TypeEQ(customfield.TypeDatetime)).
			Select(customfield.FieldKey).
			Scan(ctx, &datetimes)
		if err != nil {
			return nil, false, err
		}
		for _, key := range datetimes {
			written = append(written, datetrigger.Field{Kind: datetrigger.CustomField, Key: key})
		}
	}
	marks := lo.Reject(held, func(f datetrigger.Field, _ int) bool { return slices.Contains(written, f) })
	if events.IsSendLocked(ctx) {
		marks = append(marks, written...)
	}
	slices.SortFunc(marks, func(a, b datetrigger.Field) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Key, b.Key))
	})
	same := len(marks) == len(held) && lo.Every(held, marks)
	return marks, !same, nil
}

// sameDay reports whether two birthdays are the same date, or both absent.
func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

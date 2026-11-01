package contacts

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
)

// publishFieldChanges records contact.field_changed for each Custom field whose value differs
// between before (the stored set, nil for a new Contact) and the Contact's set now (#897). A field
// given the value it held records nothing; a field that has a value where it had none, or none
// where it had one, counts, and a cleared one is recorded without a value. Keys are recorded in
// order, so a write of several fields publishes them alike every time.
func publishFieldChanges(ctx context.Context, pub events.Publisher, workspaceID int64, c *ent.Contact, before map[string]any) error {
	after := c.CustomFields
	keys := lo.Uniq(append(lo.Keys(before), lo.Keys(after)...))
	slices.Sort(keys)
	for _, key := range keys {
		was, now := storedValue(before[key]), storedValue(after[key])
		if reflect.DeepEqual(was, now) {
			continue
		}
		if err := pub.Publish(ctx, &events.ContactFieldChanged{
			WorkspaceID: workspaceID, ContactID: c.ID, Email: lo.FromPtr(c.Email), Key: key, Value: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// storedValue is a Custom field value as it reads back from the stored JSON, so a value given as
// 4, 4.0 or json.Number("4") is one value: nil for a value that is absent or null.
func storedValue(v any) any {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}

package schema

import "fmt"

// jsonCheck is the CHECK that a column holding a document the app decodes is JSON of the form
// given ("ARRAY", "OBJECT"), so the database refuses text that no decoder reads. A NULL passes.
// The column stays text, holding the document as the app wrote it: an empty Condition is ”, which
// jsonb cannot hold, and an audited change compares the stored text with the written one.
func jsonCheck(column, form string) string {
	return fmt.Sprintf("%s IS JSON %s", column, form)
}

// conditionCheck is jsonCheck for a Condition (internal/domain/contacts/condition), a JSON
// group, which may also be ”: an empty Condition, matching every Contact.
func conditionCheck(column string) string {
	return fmt.Sprintf("%s = '' OR %s", column, jsonCheck(column, "OBJECT"))
}

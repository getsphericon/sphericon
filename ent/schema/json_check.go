package schema

import (
	"encoding/json"
	"fmt"

	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/samber/lo"
)

// jsonCheck is the CHECK that a jsonb column holding a document the app decodes is of the JSON type
// given ("array", "object"), so the database refuses a document that no decoder reads (ADR 0169). A
// NULL passes.
func jsonCheck(column, jsonType string) string {
	return fmt.Sprintf("jsonb_typeof(%s) = '%s'", column, jsonType)
}

// conditionCheck is jsonCheck for a Condition (conditiontree.Segment, conditiontree.Rule), a JSON object.
func conditionCheck(column string) string {
	return jsonCheck(column, "object")
}

// conditionEntriesCheck is the CHECK that a Condition's group holds its entries as a list (ADR
// 0181): a tree without them, or with null ones, would read as the empty group, which matches every
// Contact. A missing key is refused too, so the comparison is never NULL; a NULL Condition passes.
func conditionEntriesCheck(column string) string {
	return fmt.Sprintf("%[1]s IS NULL OR COALESCE(jsonb_typeof(%[1]s -> 'entries') = 'array', false)", column)
}

// emptyConditionSQL is the empty Condition (conditiontree.Empty) as the default a column gives a
// row written without one: the group with no tests, which matches every Contact.
var emptyConditionSQL = string(lo.Must(json.Marshal(conditiontree.Empty())))

// stepsCheck is the CHECK that an Automation version's steps are a path (automationstep.Path, ADR
// 0122): an object with its Steps as a list. The array a release before the path writes, and a path
// without its list, are refused, so the comparison is never NULL.
func stepsCheck(column string) string {
	return fmt.Sprintf("COALESCE(jsonb_typeof(%[1]s -> 'steps') = 'array', false)", column)
}

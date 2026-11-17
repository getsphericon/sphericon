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

// conditionCheck is jsonCheck for a Condition (conditiontree.Group), a JSON object.
func conditionCheck(column string) string {
	return jsonCheck(column, "object")
}

// emptyConditionSQL is the empty Condition (conditiontree.Empty) as the default a column gives a
// row written without one: the group with no tests, which matches every Contact.
var emptyConditionSQL = string(lo.Must(json.Marshal(conditiontree.Empty())))

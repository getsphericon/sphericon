package schema

import (
	"fmt"
	"slices"
	"strings"
)

// variant names the columns that belong to one value of a discriminator column (a Broadcast's
// channel, an Outbound message's kind): required ones are set on a row of that value, and none of
// them is set on a row of another value unless that value has the column too.
type variant struct {
	value              string
	required, optional []string
}

// variantCheck is the CHECK that a row holds exactly the columns of its value of column: one arm
// per variant, each requiring that variant's required columns and leaving unset every column of
// another variant that is not one of its own. A table that keeps several kinds of row in one
// table states which columns each kind owns here, so the database refuses a row that mixes them.
func variantCheck(column string, variants []variant) string {
	arms := make([]string, len(variants))
	for i, v := range variants {
		conds := []string{fmt.Sprintf("%s = '%s'", column, v.value)}
		for _, col := range v.required {
			conds = append(conds, col+" IS NOT NULL")
		}
		own := slices.Concat(v.required, v.optional)
		var unset []string
		for _, other := range variants {
			if other.value == v.value {
				continue
			}
			for _, col := range slices.Concat(other.required, other.optional) {
				if !slices.Contains(own, col) && !slices.Contains(unset, col) {
					unset = append(unset, col)
				}
			}
		}
		for _, col := range unset {
			conds = append(conds, col+" IS NULL")
		}
		arms[i] = "(" + strings.Join(conds, " AND ") + ")"
	}
	return strings.Join(arms, " OR ")
}

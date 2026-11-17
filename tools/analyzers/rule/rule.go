// Package rule is the one place that words and reports an analyzer's finding, so every message keeps the
// contract of docs/standards/review-rules.md: it begins with the rule number and ends with how to
// fix the violation.
package rule

import (
	"fmt"
	"go/token"

	"golang.org/x/tools/go/analysis"
)

// Message is "rule <n>: <what is wrong>; fix: <what to do instead>".
func Message(n int, what, fix string) string {
	return fmt.Sprintf("rule %d: %s; fix: %s", n, what, fix)
}

// Report reports a finding of rule n at pos, worded by Message.
func Report(pass *analysis.Pass, pos token.Pos, n int, what, fix string) {
	pass.Reportf(pos, "%s", Message(n, what, fix))
}

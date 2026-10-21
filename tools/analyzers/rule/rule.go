// Package rule is the one place that words an analyzer's finding, so every message keeps the
// contract of docs/agents/review-rules.md: it begins with the rule number and ends with how to
// fix the violation.
package rule

import "fmt"

// Message is "rule <n>: <what is wrong>; fix: <what to do instead>".
func Message(n int, what, fix string) string {
	return fmt.Sprintf("rule %d: %s; fix: %s", n, what, fix)
}

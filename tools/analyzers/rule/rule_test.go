package rule_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

func TestMessageBeginsWithTheRuleAndEndsWithTheFix(t *testing.T) {
	got := rule.Message(5, "a handler holds logic", "move it into a domain module")

	want := "rule 5: a handler holds logic; fix: move it into a domain module"
	assert.Equal(t, want, got)
}

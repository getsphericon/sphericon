package resources_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// A Step whose view has no arm of the union fails the response: it is never shown as an empty
// Step.
func TestAStepViewWithNoArmFailsTheResponse(t *testing.T) {
	for name, view := range map[string]automations.StepView{
		"no view":              nil,
		"a tag view of a wait": automations.TagView{ID: "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1b04", Type: automationstep.KindWait},
	} {
		_, err := (&resources.ConverterImpl{}).AutomationPathToResource(automations.PathView{Steps: []automations.StepView{view}})

		assert.Error(t, err, name)
	}
}

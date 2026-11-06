package versioneddefinition_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/content/versioneddefinition"
)

// Every refusal of a kind is its own: a lifecycle built without one would answer a move with no
// refusal, or a bare error the server turns into a 500, so New refuses to build it.
func TestNewRefusesAKindThatNamesNotEveryRefusal(t *testing.T) {
	refusal := errors.New("refused")
	every := versioneddefinition.Errors{NotFound: refusal, Archived: refusal, NothingToPublish: refusal, NothingToRevert: refusal}
	for _, missing := range []struct {
		name string
		errs versioneddefinition.Errors
	}{
		{"NotFound", versioneddefinition.Errors{Archived: refusal, NothingToPublish: refusal, NothingToRevert: refusal}},
		{"Archived", versioneddefinition.Errors{NotFound: refusal, NothingToPublish: refusal, NothingToRevert: refusal}},
		{"NothingToPublish", versioneddefinition.Errors{NotFound: refusal, Archived: refusal, NothingToRevert: refusal}},
		{"NothingToRevert", versioneddefinition.Errors{NotFound: refusal, Archived: refusal, NothingToPublish: refusal}},
	} {
		t.Run(missing.name, func(t *testing.T) {
			assert.Panics(t, func() {
				versioneddefinition.New[ent.Automation, ent.AutomationVersion, any, any](nil, nil, missing.errs)
			})
		})
	}
	assert.NotPanics(t, func() { versioneddefinition.New[ent.Automation, ent.AutomationVersion, any, any](nil, nil, every) })
}

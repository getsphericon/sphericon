package drafts_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/content/drafts"
)

// Every refusal of a kind is its own: a lifecycle built without one would answer a move with no
// refusal, or a bare error the server turns into a 500, so New refuses to build it.
func TestNewRefusesAKindThatNamesNotEveryRefusal(t *testing.T) {
	refusal := errors.New("refused")
	every := drafts.Errors{NotFound: refusal, Archived: refusal, NothingToPublish: refusal, NothingToRevert: refusal}
	for _, missing := range []struct {
		name string
		errs drafts.Errors
	}{
		{"NotFound", drafts.Errors{Archived: refusal, NothingToPublish: refusal, NothingToRevert: refusal}},
		{"Archived", drafts.Errors{NotFound: refusal, NothingToPublish: refusal, NothingToRevert: refusal}},
		{"NothingToPublish", drafts.Errors{NotFound: refusal, Archived: refusal, NothingToRevert: refusal}},
		{"NothingToRevert", drafts.Errors{NotFound: refusal, Archived: refusal, NothingToPublish: refusal}},
	} {
		t.Run(missing.name, func(t *testing.T) {
			assert.Panics(t, func() { drafts.New[ent.Automation, ent.AutomationVersion, any, any](nil, nil, missing.errs) })
		})
	}
	assert.NotPanics(t, func() { drafts.New[ent.Automation, ent.AutomationVersion, any, any](nil, nil, every) })
}

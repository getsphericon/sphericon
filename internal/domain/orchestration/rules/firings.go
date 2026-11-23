package rules

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rulefiring"
)

// Firings reads the Rule Firings for the modules that act on them: the Automations ask which Event
// the Firing whose Enroll Action made an Enrollment fired on, which never ends that Enrollment (ADR
// 0133).
type Firings struct{}

// NewFirings builds the reader. It needs nothing: every question reads in the scope it is given.
func NewFirings() *Firings { return &Firings{} }

// SourceOf is the source id of the Event the Firing fired on, "" when the Workspace has no such
// Firing any more (the Event retention removed it with its Event).
func (f *Firings) SourceOf(ctx context.Context, s *ent.Scoped, firingID int64) (string, error) {
	firing, err := s.RuleFiring().Query().Where(rulefiring.ID(firingID)).Only(ctx)
	if ent.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read rule firing %d: %w", firingID, err)
	}
	return firing.SourceID, nil
}

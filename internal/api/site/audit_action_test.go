package site_test

import (
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
)

// The contract's AuditAction is exactly what the server records in a Workspace's Audit log: every
// explicit action, and the actions of every audited entity (its create, update and delete, and its
// discard and restore when it is archived). A new action, on either side, fails here until the
// other side has it too, so no entry reaches a reader whose client does not know its action.
func TestTheContractDeclaresEveryActionTheAuditLogRecords(t *testing.T) {
	recorded := slices.Concat(events.ExplicitAuditActions, lo.FlatMap(ent.ScopedEntities(), func(e ent.ScopedEntity, _ int) []string {
		return e.AuditActions
	}))

	declared := lo.Map(siteapi.AuditAction("").AllValues(), func(a siteapi.AuditAction, _ int) string { return string(a) })

	assert.ElementsMatch(t, recorded, declared)
}

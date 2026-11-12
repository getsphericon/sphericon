package staff_test

import (
	"testing"

	"github.com/samber/lo"

	"github.com/stretchr/testify/assert"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/domain/instance/instanceauditentries"
)

// The contract's InstanceAuditAction is exactly what the Instance audit log records.
func TestTheContractDeclaresEveryActionTheInstanceAuditLogRecords(t *testing.T) {
	assert.ElementsMatch(t,
		lo.Map(instanceauditentries.Actions, func(a string, _ int) staffapi.InstanceAuditAction { return staffapi.InstanceAuditAction(a) }),
		staffapi.InstanceAuditAction("").AllValues())
}

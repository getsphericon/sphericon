package staff_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/instanceaudit"
)

// The contract's InstanceAuditAction is exactly what the Instance audit log records.
func TestTheContractDeclaresEveryActionTheInstanceAuditLogRecords(t *testing.T) {
	assert.ElementsMatch(t,
		[]staffapi.InstanceAuditAction{instanceaudit.ActionDefaultSendCapChange},
		staffapi.InstanceAuditAction("").AllValues())
}

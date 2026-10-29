package secondfactor_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/secondfactor"
)

const resetEmail = "sam@globex.test"

func TestReportResetSaysTheFactorIsClearedAndSessionsEnded(t *testing.T) {
	var out bytes.Buffer

	err := secondfactor.ReportReset(&out, resetEmail, true, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "the factor and recovery codes are cleared and every session has ended")
}

func TestReportResetOfAUserWithoutASecondFactorSaysNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := secondfactor.ReportReset(&out, resetEmail, false, nil)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "has no second factor; nothing changed")
}

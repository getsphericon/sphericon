package instancestates_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
)

// An Instance reports at a fixed time of day: the same offset for its id every time, within the
// six hours the reports are spread over.
func TestReportOffsetIsAStableTimeOfDayDerivedFromTheInstanceID(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		id := rapid.String().Draw(rt, "id")

		first, second := instancestates.ReportOffset(id), instancestates.ReportOffset(id)

		assert.Equalf(rt, first, second, "%q: the offset is stable", id)
		assert.GreaterOrEqual(rt, first, time.Duration(0))
		assert.Less(rt, first, 6*time.Hour)
	})
}

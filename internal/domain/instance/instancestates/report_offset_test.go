package instancestates_test

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
)

// An Instance reports at a fixed time of day: the same offset for its id every time, within the
// six hours the reports are spread over.
func TestReportOffsetIsAStableTimeOfDayDerivedFromTheInstanceID(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		id := rapid.String().Draw(rt, "id")

		first, second := instancestates.ReportOffset(id), instancestates.ReportOffset(id)

		if first != second || first < 0 || first >= 6*time.Hour {
			rt.Fatalf("%q: offsets %v and %v, want one stable offset in [0, 6h)", id, first, second)
		}
	})
}

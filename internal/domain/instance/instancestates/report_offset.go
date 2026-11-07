package instancestates

import (
	"hash/fnv"
	"time"
)

// reportSpread is the length of the window an Instance's Telemetry report time is spread over,
// so Instances that started together do not all report in the same minute.
const reportSpread = 6 * time.Hour

// ReportOffset is how far into the day (UTC) the Instance with instanceID sends its Telemetry
// report: a fixed 0 to 6 hours derived from its Instance id, so it is the same every day and
// after every restart.
func ReportOffset(instanceID string) time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(instanceID))
	return (time.Duration(h.Sum64() % uint64(reportSpread))).Truncate(time.Second)
}

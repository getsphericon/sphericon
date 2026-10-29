// Package sustained decides when a reading that keeps coming back over a limit counts: only
// after a run of polls in a row. The document renderer's memory guard reads a process-wide heap
// counter, so a passing burst of another goroutine's allocations reads as growth too; a run that
// a poll back under the limit breaks never stops a call. The type is pure (no clock, no heap) so
// the decision is tested on its inputs.
package sustained

// Counter counts polls in a row that read over the limit. Build one with New.
type Counter struct{ over, need int }

// New is a counter that fires once need polls in a row read over the limit.
func New(need int) *Counter { return &Counter{need: need} }

// Observe records one poll and says whether the limit has now been over for need polls in a row.
// A poll back under the limit starts the count again.
func (c *Counter) Observe(over bool) bool {
	if !over {
		c.over = 0
		return false
	}
	c.over++
	return c.over >= c.need
}

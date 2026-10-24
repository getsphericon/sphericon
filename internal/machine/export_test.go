package machine

import "time"

// SetClock replaces the detector's clock, so a test moves time past the reload interval.
func (d *Detector) SetClock(now func() time.Time) { d.now = now }

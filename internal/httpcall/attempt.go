package httpcall

import "time"

// Attempt is the execution history a call's owner needs to decide whether to retry. The queue
// adapter supplies ordinary data; executing a call does not require a queue job.
type Attempt struct {
	Number    int
	Limit     int
	CreatedAt time.Time
	Now       time.Time
}

// Package drain removes a backlog of rows in bounded batches: a job or an erase that deletes rows
// a batch at a time, so no single statement holds a long lock, runs its batch through Batches.
package drain

// Batches runs batch until one removes fewer than size rows, and returns how many rows the runs
// removed in all. On an error it stops and returns what the runs before it removed.
func Batches(size int, batch func() (int, error)) (int, error) {
	total := 0
	for {
		removed, err := batch()
		if err != nil {
			return total, err
		}
		total += removed
		if removed < size {
			return total, nil
		}
	}
}

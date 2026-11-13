package testhelper

import "time"

// DeletionWindow is ADR 0043's window, how long a deleted Workspace stays restorable before it is
// erased. The tests that put a Workspace before or past it state it here rather than read it from
// the code under test (review rule 11).
const DeletionWindow = 30 * 24 * time.Hour

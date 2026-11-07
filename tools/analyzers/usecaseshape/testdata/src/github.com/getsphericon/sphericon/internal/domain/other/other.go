// Package other is a domain module: the call contract of the use cases does not bind it.
package other

import (
	"context"
	"time"
)

func Operation(context.Context, string, string, string) (int, int, error) { return 0, 0, nil }

func Stamp() time.Time { return time.Now() }

// Package exempt breaks the call contract but is on the exemption list: nothing is reported.
package exempt

import "context"

func Operation(context.Context, string, string, string) (int, int, error) { return 0, 0, nil }

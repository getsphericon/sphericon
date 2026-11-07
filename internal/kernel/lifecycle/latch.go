package lifecycle

import "context"

// Latch records a fact once (ADR 0109). The write matches only while its moment is null,
// alongside the owner's other conditions, and returns whether it stored the moment.
// Like a machine's Storage.Write, a lost conditional write is ErrInvalidTransition.
// There is no operation to clear a latch.
func Latch(ctx context.Context, write func(context.Context) (bool, error)) error {
	swapped, err := write(ctx)
	if err != nil {
		return err
	}
	if !swapped {
		return ErrInvalidTransition
	}
	return nil
}

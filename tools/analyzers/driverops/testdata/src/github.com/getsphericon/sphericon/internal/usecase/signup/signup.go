package signup

import "context"

type UseCase struct{}

func (*UseCase) Complete(ctx context.Context, email string) error { return nil }

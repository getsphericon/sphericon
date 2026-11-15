package userdeletion

import "context"

type UseCase struct{}

func (*UseCase) Delete(ctx context.Context, email string) error { return nil }

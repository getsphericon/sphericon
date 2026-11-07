package versioneddefinition

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The lifecycle writes through the store the owner bound: the owner's Pointers is the write.
func edit[O, V any](ctx context.Context, store ent.VersionedStore[O, V]) error {
	return store.CompareAndSwap(ctx, 1, 2)
}

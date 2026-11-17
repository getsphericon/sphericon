// Package borrowed builds another use case's UseCase: its New returns no *UseCase of its own.
package borrowed

import "github.com/getsphericon/sphericon/internal/usecase/clean"

func New() *clean.UseCase { return clean.New(nil) } // want `rule 5: New returns something other than \*UseCase`

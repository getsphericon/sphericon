package eebad

import (
	"context"
	"time"
)

type UseCase struct{}

func New() (*UseCase, error) { return &UseCase{}, nil } // want `rule 5: New returns something other than \*UseCase`

func Prune(context.Context) error { return nil } // want `rule 5: Prune is an exported function of a use case package`

// Detector is a service type beside UseCase.
type Detector struct{}

func (*Detector) Run(context.Context) error { return nil } // want `rule 5: Run is a method of Detector, an exported type other than UseCase`

// Summary is a value type: its methods take no context.
type Summary struct{ N int }

func (s Summary) Total() int { return s.N }

func (u *UseCase) WithClock(func() time.Time) *UseCase { return u } // want `rule 5: WithClock returns \*UseCase`

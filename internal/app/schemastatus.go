package app

import (
	"context"
	"encoding/json"
	"io"

	"github.com/getsphericon/sphericon/internal/domain/instance/upgrade"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

// WriteSchemaBoundaries is `sphericon migrate --status --json`: what the Major walk of `upgrade`
// reads from the release it is about to install, the database's state and this binary's table of
// Majors (upgrade.Boundaries). It changes nothing.
func WriteSchemaBoundaries(ctx context.Context, url string, w io.Writer) error {
	st, err := db.ReadStatus(ctx, url)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(boundaries(st))
}

func boundaries(st db.SchemaStatus) upgrade.Boundaries {
	b := upgrade.Boundaries{State: upgrade.MajorFloor{Major: st.State.Major, Floor: st.State.Floor}}
	for _, m := range db.Majors {
		b.Majors = append(b.Majors, upgrade.MajorFloor{Major: m.Major, Floor: m.Floor})
	}
	return b
}

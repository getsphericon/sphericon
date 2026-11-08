package commandline

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"

	"github.com/getsphericon/sphericon/internal/domain/instance/upgrade"
)

// ErrNewerRelease is what `upgrade --check` returns once it has said that a newer release
// exists; the command exits 10 on it (ADR 0034).
var ErrNewerRelease = errors.New("a newer release is available")

// WriteUpgrade prints each Progress of `sphericon upgrade` as it comes (upgrade.Run), and
// returns the error the Upgrade ended with, or ErrNewerRelease after a check that found one.
func WriteUpgrade(w io.Writer, progress iter.Seq2[upgrade.Progress, error]) error {
	newer := false
	for p, err := range progress {
		if err != nil {
			return err
		}
		if latest, ok := p.(upgrade.Latest); ok && latest.Newer {
			newer = true
		}
		if _, err := io.WriteString(w, upgradeLine(p)); err != nil {
			return err
		}
	}
	if newer {
		return ErrNewerRelease
	}
	return nil
}

// upgradeLine is what one Progress prints.
func upgradeLine(p upgrade.Progress) string {
	switch p := p.(type) {
	case upgrade.Latest:
		if p.Newer {
			return fmt.Sprintf("A newer release is available: %s (running %s). Run `sphericon upgrade`.\n", p.Release, p.Running)
		}
		return fmt.Sprintf("Up to date: %s is the latest release.\n", p.Release)
	case upgrade.AlreadyRunning:
		return fmt.Sprintf("Already running %s.\n", p.Version)
	case upgrade.Crossing:
		return fmt.Sprintf("Crossing %d Majors: installing %s in turn.\n", p.Majors, strings.Join(p.Plan, ", "))
	case upgrade.Notes:
		return fmt.Sprintf("Upgrade notes between %s and %s:\n\n%s\n", p.From, p.To, p.Text)
	case upgrade.Migrated:
		return p.Output
	case upgrade.Installed:
		if p.Restarted {
			return fmt.Sprintf("Installed %s at %s and restarted %s.\n", p.Version, p.Binary, p.Service)
		}
		return fmt.Sprintf("Installed %s at %s (the previous binary is %s).\nRestart to run it: systemctl restart %s\n", p.Version, p.Binary, p.Old, p.Service)
	}
	return ""
}

package commandline_test

import (
	"bytes"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/instance/upgrade"
)

// upgradeOf is an Upgrade that yields progress, then ends with end (nil: it succeeded).
func upgradeOf(end error, progress ...upgrade.Progress) iter.Seq2[upgrade.Progress, error] {
	return func(yield func(upgrade.Progress, error) bool) {
		for _, p := range progress {
			if !yield(p, nil) {
				return
			}
		}
		if end != nil {
			yield(nil, end)
		}
	}
}

func TestWriteUpgradeOfACurrentInstanceSaysItIsUpToDate(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteUpgrade(&out, upgradeOf(nil, upgrade.Latest{Release: "0.2.0", Running: "v0.2.0"}))

	require.NoError(t, err)
	assert.Equal(t, "Up to date: 0.2.0 is the latest release.\n", out.String())
}

func TestWriteUpgradeOfANewerReleaseSaysSoAndSignalsIt(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteUpgrade(&out, upgradeOf(nil, upgrade.Latest{Release: "0.3.0", Running: "v0.2.0", Newer: true}))

	require.ErrorIs(t, err, commandline.ErrNewerRelease)
	assert.Equal(t, "A newer release is available: 0.3.0 (running v0.2.0). Run `sphericon upgrade`.\n", out.String())
}

func TestWriteUpgradeShowsTheNotesBeforeTheRefusalToGoOn(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteUpgrade(&out, upgradeOf(upgrade.ErrNeedsConfirmation,
		upgrade.Crossing{Majors: 1, Plan: []string{"0.3.2", "0.6.0"}},
		upgrade.Notes{From: "v0.1.0", To: "0.6.0", Text: "0.3.2\n  - export your segments\n"}))

	require.ErrorIs(t, err, upgrade.ErrNeedsConfirmation)
	assert.Equal(t, "Crossing 1 Majors: installing 0.3.2, 0.6.0 in turn.\n"+
		"Upgrade notes between v0.1.0 and 0.6.0:\n\n0.3.2\n  - export your segments\n\n", out.String())
}

func TestWriteUpgradeNamesTheRestartLeftToTheSelfHoster(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteUpgrade(&out, upgradeOf(nil,
		upgrade.Migrated{Version: "0.2.0"},
		upgrade.Installed{Result: upgrade.Result{Version: "0.2.0", Binary: "/usr/local/bin/sphericon", Old: "/usr/local/bin/sphericon.old"}, Service: "sphericon-prod"}))

	require.NoError(t, err)
	assert.Equal(t, "Installed 0.2.0 at /usr/local/bin/sphericon (the previous binary is /usr/local/bin/sphericon.old).\n"+
		"Restart to run it: systemctl restart sphericon-prod\n", out.String())
}

func TestWriteUpgradeOfARestartSaysTheServiceRestarted(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteUpgrade(&out, upgradeOf(nil,
		upgrade.Installed{Result: upgrade.Result{Version: "0.2.0", Binary: "/usr/local/bin/sphericon"}, Service: "sphericon", Restarted: true}))

	require.NoError(t, err)
	assert.Equal(t, "Installed 0.2.0 at /usr/local/bin/sphericon and restarted sphericon.\n", out.String())
}

func TestWriteUpgradePrintsWhatTheMigrationPrintedAndThatNothingChanged(t *testing.T) {
	var out bytes.Buffer

	err := commandline.WriteUpgrade(&out, upgradeOf(nil, upgrade.Migrated{Version: "0.2.0", Output: "applied 3\n"}, upgrade.AlreadyRunning{Version: "0.2.0"}))

	require.NoError(t, err)
	assert.Equal(t, "applied 3\nAlready running 0.2.0.\n", out.String())
}

func TestWriteUpgradeOfAFailedStepShowsWhatCameBefore(t *testing.T) {
	failed := errors.New("migrate with 0.5.1 (nothing was swapped): exit status 3")
	var out bytes.Buffer

	err := commandline.WriteUpgrade(&out, upgradeOf(failed,
		upgrade.Installed{Result: upgrade.Result{Version: "0.3.2", Binary: "/bin/sphericon", Old: "/bin/sphericon.old"}, Service: "sphericon"}))

	require.ErrorIs(t, err, failed)
	assert.Contains(t, out.String(), "Installed 0.3.2")
}

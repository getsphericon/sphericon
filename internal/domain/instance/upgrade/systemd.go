package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Systemd is `upgrade --restart`: it runs the new binary's `migrate` as a transient unit
// with the service's own User and EnvironmentFile (so the Self-hoster installs no second
// unit file, ADR 0034), and restarts the service once the binary is swapped.
type Systemd struct {
	Service string // unit name, "sphericon"
	Out     io.Writer
}

var environmentFile = regexp.MustCompile(`(\S+) \(ignore_errors=(yes|no)\)`)

// Run is Options.Run: `migrate` (and `migrate --status --json`, which reads the database)
// goes through systemd-run, everything else runs directly.
func (s Systemd) Run(ctx context.Context, bin string, args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "migrate" {
		return execRun(ctx, bin, args, stdout)
	}
	props, err := s.properties(ctx)
	if err != nil {
		return err
	}
	return execRun(ctx, "systemd-run", append(props, append([]string{bin}, args...)...), stdout)
}

// properties reads the service's User and EnvironmentFiles into systemd-run arguments.
func (s Systemd) properties(ctx context.Context) ([]string, error) {
	var out bytes.Buffer
	if err := execRun(ctx, "systemctl", []string{"show", "--property=User", "--property=EnvironmentFiles", s.Service}, &out); err != nil {
		return nil, fmt.Errorf("read the unit %s: %w", s.Service, err)
	}
	args := []string{"--wait", "--collect", "--pipe"}
	for line := range strings.SplitSeq(out.String(), "\n") {
		switch key, value, _ := strings.Cut(line, "="); key {
		case "User":
			if value != "" {
				args = append(args, "-p", "User="+value)
			}
		case "EnvironmentFiles":
			for _, m := range environmentFile.FindAllStringSubmatch(value, -1) {
				optional := ""
				if m[2] == "yes" {
					optional = "-"
				}
				args = append(args, "-p", "EnvironmentFile="+optional+m[1])
			}
		}
	}
	return args, nil
}

// PostSwap is Options.PostSwap: it restarts the service and fails when it is not active.
func (s Systemd) PostSwap(ctx context.Context, r Result) error {
	if err := execRun(ctx, "systemctl", []string{"restart", s.Service}, io.Discard); err != nil {
		return fmt.Errorf("restart %s (the previous binary is %s): %w", s.Service, r.Old, err)
	}
	var state bytes.Buffer
	if err := execRun(ctx, "systemctl", []string{"is-active", s.Service}, &state); err != nil {
		return fmt.Errorf("%s is %s after the restart (the previous binary is %s): see `journalctl -u %s`", s.Service, strings.TrimSpace(state.String()), r.Old, s.Service)
	}
	_, err := fmt.Fprintf(s.Out, "Installed %s at %s and restarted %s.\n", r.Version, r.Binary, s.Service)
	return err
}

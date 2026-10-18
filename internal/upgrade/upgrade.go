// Package upgrade replaces a bare-binary Instance's binary with a newer release without
// stopping it (ADR 0033, ADR 0034): it verifies the archive, unpacks the new binary next to
// the old one, runs the new binary's `migrate` while the old one still serves, and only then
// swaps the file atomically, keeping the old one as `.old`.
package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
)

// Errors a caller maps to an exit code or a message.
var (
	// ErrNeedsConfirmation: the release has Upgrade notes and --yes was not given.
	ErrNeedsConfirmation = errors.New("the release has Upgrade notes: read them above and rerun with --yes")
	// ErrDowngrade: the requested release is older than the running one.
	ErrDowngrade = errors.New("refusing to downgrade")
)

const maxBinary = 1 << 30

// Options is one Upgrade step.
type Options struct {
	Source   Source
	Target   string   // path of the installed binary
	Current  string   // running version, "v0.1.0" or "0.1.0"
	Version  string   // pinned release; empty = the newest the Source has
	Platform Platform // zero = this OS and architecture
	Yes      bool     // go past Upgrade notes
	Out      io.Writer

	// Container names the command to use instead when running inside a container or on
	// Kubernetes ("docker" or "helm"), "" on a bare host; nil = detect from the environment.
	Container func() string
	// Run executes a binary, writing its stdout to stdout; nil = os/exec with the
	// process environment and stderr passed through.
	Run func(ctx context.Context, bin string, args []string, stdout io.Writer) error
	// PostSwap runs after the swap; nil = print the restart command. A later `--restart`
	// replaces it with one that restarts the systemd service.
	PostSwap func(ctx context.Context, r Result) error
}

// Result is what a step left on disk.
type Result struct {
	Changed bool   // false when the Instance already runs the release
	Version string // installed release, without the leading "v"
	Binary  string // the installed path
	Old     string // the previous binary, kept next to it
}

// Step runs one Upgrade: to the pinned release, or to the newest the Source has.
func Step(ctx context.Context, o Options) (Result, error) {
	if o.Platform == (Platform{}) {
		o.Platform = Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	}
	if o.Container == nil {
		o.Container = DetectContainer
	}
	if o.Run == nil {
		o.Run = execRun
	}
	if o.PostSwap == nil {
		o.PostSwap = func(_ context.Context, r Result) error { return PrintRestart(o.Out, r) }
	}
	if kind := o.Container(); kind != "" {
		return Result{}, containerRefusal(kind)
	}
	current := "v" + strings.TrimPrefix(o.Current, "v")
	if !semver.IsValid(current) {
		return Result{}, fmt.Errorf("this build is %q, not a release: install a release binary first", o.Current)
	}

	version, err := chooseVersion(ctx, o)
	if err != nil {
		return Result{}, err
	}
	switch semver.Compare("v"+version, current) {
	case -1:
		return Result{}, fmt.Errorf("%w: %s is older than %s", ErrDowngrade, version, o.Current)
	case 0:
		_, err := fmt.Fprintf(o.Out, "Already running %s.\n", version)
		return Result{Version: version, Binary: o.Target}, err
	}

	rel, err := o.Source.Fetch(ctx, version, o.Platform)
	if err != nil {
		return Result{}, err
	}
	if rel.Cleanup != nil {
		defer rel.Cleanup()
	}
	if err := verify(rel); err != nil {
		return Result{}, err
	}

	staged := o.Target + ".new"
	if err := unpack(rel.ArchivePath, staged); err != nil {
		_ = os.Remove(staged)
		return Result{}, err
	}
	res, err := install(ctx, o, current, staged, version)
	if err != nil {
		_ = os.Remove(staged)
		return Result{}, err
	}
	return res, o.PostSwap(ctx, res)
}

func install(ctx context.Context, o Options, current, staged, version string) (Result, error) {
	var notes bytes.Buffer
	if err := o.Run(ctx, staged, []string{"upgrade-notes", "--since", current}, &notes); err != nil {
		return Result{}, fmt.Errorf("read the Upgrade notes of %s: %w", version, err)
	}
	if notes.Len() > 0 {
		if _, err := fmt.Fprintf(o.Out, "Upgrade notes between %s and %s:\n\n%s\n", current, version, notes.String()); err != nil {
			return Result{}, err
		}
		if !o.Yes {
			return Result{}, ErrNeedsConfirmation
		}
	}
	if err := o.Run(ctx, staged, []string{"migrate"}, o.Out); err != nil {
		return Result{}, fmt.Errorf("migrate with %s (nothing was swapped): %w", version, err)
	}
	old := o.Target + ".old"
	if err := swap(o.Target, staged, old); err != nil {
		return Result{}, err
	}
	return Result{Changed: true, Version: version, Binary: o.Target, Old: old}, nil
}

// swap keeps target as old and puts staged in its place; the last step is one rename, so the
// binary is never missing.
func swap(target, staged, old string) error {
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Link(target, old); err != nil {
		return fmt.Errorf("keep the old binary as %s: %w", old, err)
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Remove(old)
		return err
	}
	return nil
}

func chooseVersion(ctx context.Context, o Options) (string, error) {
	if o.Version != "" {
		v := strings.TrimPrefix(o.Version, "v")
		if !semver.IsValid("v" + v) {
			return "", fmt.Errorf("%q is not a version", o.Version)
		}
		return v, nil
	}
	versions, err := o.Source.Versions(ctx, o.Platform)
	if err != nil {
		return "", err
	}
	newest := ""
	for _, v := range versions {
		if semver.IsValid("v"+v) && (newest == "" || semver.Compare("v"+v, "v"+newest) > 0) {
			newest = v
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no release for %s/%s found", o.Platform.OS, o.Platform.Arch)
	}
	return newest, nil
}

// verify checks the archive's sha256 against its line in checksums.txt.
func verify(rel Release) error {
	sums, err := os.ReadFile(rel.ChecksumsPath)
	if err != nil {
		return err
	}
	want := ""
	for line := range strings.SplitSeq(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.TrimPrefix(f[1], "*") == rel.ArchiveName {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("checksum: %s has no entry in checksums.txt", rel.ArchiveName)
	}
	f, err := os.Open(rel.ArchivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", rel.ArchiveName, want, got)
	}
	return nil
}

// unpack writes the archive's sphericon binary to dst.
func unpack(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s has no sphericon binary", filepath.Base(archive))
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg || path.Base(h.Name) != "sphericon" {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, io.LimitReader(tr, maxBinary))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		return err
	}
}

// DetectContainer says "docker" inside a container and "helm" on Kubernetes, "" otherwise.
func DetectContainer() string {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "helm"
	}
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return "docker"
		}
	}
	return ""
}

func containerRefusal(kind string) error {
	if kind == "helm" {
		return errors.New("this Instance runs on Kubernetes: upgrade it with `helm upgrade sphericon oci://ghcr.io/getsphericon/charts/sphericon --version <version> --reuse-values`, not by replacing a binary in an image")
	}
	return errors.New("this Instance runs in a container: upgrade it with `docker pull ghcr.io/getsphericon/sphericon:<version>` and recreate the container, not by replacing a binary in an image")
}

// PrintRestart is the default post-swap step: it names the one command left to the user.
func PrintRestart(w io.Writer, r Result) error {
	_, err := fmt.Fprintf(w, "Installed %s at %s (the previous binary is %s).\nRestart to run it: systemctl restart sphericon\n", r.Version, r.Binary, r.Old)
	return err
}

func execRun(ctx context.Context, bin string, args []string, stdout io.Writer) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

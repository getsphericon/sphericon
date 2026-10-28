package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/preset"
)

// presetCommand groups the commands on Presets (ADR 0037).
func presetCommand() *cli.Command {
	return group("preset", "work with Presets, the declarative description of a provider's callbacks",
		presetTestCommand(), presetSchemaCommand())
}

// presetSchemaCommand prints the JSON Schema of the Preset file, for an editor.
func presetSchemaCommand() *cli.Command {
	return &cli.Command{
		Name:  "schema",
		Usage: "print the JSON Schema of the Preset file",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
			}
			_, err := out(cmd).Write(preset.Schema)
			return err
		},
	}
}

// presetTestCommand runs a Preset's own tests, or verification and transform on a
// sample, before a Self-hoster points a provider at the file.
func presetTestCommand() *cli.Command {
	return &cli.Command{
		Name:      "test",
		Usage:     "run a Preset's tests, or its verification and transform on a sample payload",
		ArgsUsage: "<file | built-in name>",
		// A --header value such as a Standard Webhooks signature holds commas.
		DisableSliceFlagSeparator: true,
		Description: "Without --payload, runs the `tests` the Preset carries and prints PASS or FAIL for each case.\n" +
			"With --payload, verifies the sample with the Preset's scheme (--header name=value and\n" +
			"--param name=value, repeatable, supply the request headers and the Integration's params,\n" +
			"secrets included) and prints the Delivery notices, or the reason it was refused.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "payload", Usage: "file holding the raw request body to run the Preset on"},
			&cli.StringSliceFlag{Name: "header", Usage: "request header of the sample, name=value"},
			&cli.StringSliceFlag{Name: "param", Usage: "Integration param of the sample, name=value"},
			&cli.StringFlag{Name: "now", Usage: "the clock for the sample, an RFC 3339 time (default: now)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return fmt.Errorf("preset test needs one Preset: a file or the name of a built-in (see `%s --help`)", cmd.FullName())
			}
			p, err := loadPreset(cmd.Args().First())
			if err != nil {
				return err
			}
			if cmd.String("payload") == "" {
				return runPresetTests(ctx, out(cmd), p)
			}
			return runPresetSample(ctx, cmd, p)
		},
	}
}

// loadPreset reads a Preset file, or finds a built-in by name when the argument is not a
// file. The file is parsed on its own: its name may not collide with a built-in here, so
// a Self-hoster can test a copy of one.
func loadPreset(arg string) (*preset.Preset, error) {
	data, err := os.ReadFile(arg)
	switch {
	case err == nil:
		return preset.Parse(filepath.Base(arg), data)
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	reg, err := preset.Load(nil)
	if err != nil {
		return nil, err
	}
	p, ok := reg.Get(arg)
	if !ok {
		return nil, fmt.Errorf("%q is neither a file nor a built-in Preset (%s)", arg, strings.Join(reg.Names(), ", "))
	}
	return p, nil
}

func runPresetTests(ctx context.Context, w io.Writer, p *preset.Preset) error {
	failed := 0
	results := p.RunTests(ctx)
	for _, r := range results {
		if r.Err != nil {
			failed++
			_, _ = fmt.Fprintf(w, "FAIL %s\n     %v\n", r.Name, r.Err)
			continue
		}
		_, _ = fmt.Fprintf(w, "PASS %s\n", r.Name)
	}
	if failed > 0 {
		return fmt.Errorf("preset %s: %d of %d cases failed", p.Name(), failed, len(results))
	}
	return nil
}

func runPresetSample(ctx context.Context, cmd *cli.Command, p *preset.Preset) error {
	body, err := os.ReadFile(cmd.String("payload"))
	if err != nil {
		return err
	}
	header := http.Header{}
	headers, err := pairs(cmd.StringSlice("header"))
	if err != nil {
		return fmt.Errorf("--header: %w", err)
	}
	for k, v := range headers {
		header.Set(k, v)
	}
	values, err := pairs(cmd.StringSlice("param"))
	if err != nil {
		return fmt.Errorf("--param: %w", err)
	}
	params := preset.Params{}
	for k, v := range values {
		params[k] = v
	}
	now := time.Now()
	if raw := cmd.String("now"); raw != "" {
		if now, err = time.Parse(time.RFC3339, raw); err != nil {
			return fmt.Errorf("--now is not an RFC 3339 time: %w", err)
		}
	}

	res, err := p.Handle(ctx, preset.Request{Header: header, Body: body}, params, now)
	if err != nil {
		_, _ = fmt.Fprintf(out(cmd), "refused: %v\n", err)
		return err
	}
	for _, f := range res.Notices {
		kind := string(f.Kind)
		if f.Kind == preset.Bounce {
			kind += map[bool]string{true: " hard", false: " soft"}[f.Hard]
		}
		_, _ = fmt.Fprintf(out(cmd), "%s %s id=%s sending_domain=%s message_id=%s reason=%q\n",
			kind, f.Destination, f.ID, f.SendingDomain, f.MessageID, f.Reason)
	}
	if len(res.Notices) == 0 {
		_, _ = fmt.Fprintln(out(cmd), "verified, no Delivery notices (an event the Preset does not handle)")
	}
	return nil
}

// pairs reads repeated name=value flags.
func pairs(items []string) (map[string]string, error) {
	m := make(map[string]string, len(items))
	for _, it := range items {
		k, v, ok := strings.Cut(it, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%q is not name=value", it)
		}
		m[k] = v
	}
	return m, nil
}

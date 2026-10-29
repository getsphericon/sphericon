package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
			p, err := preset.Open(cmd.Args().First())
			if err != nil {
				return err
			}
			if cmd.String("payload") == "" {
				return p.WriteTests(ctx, out(cmd))
			}
			req, params, now, err := sample(cmd)
			if err != nil {
				return err
			}
			return p.WriteSample(ctx, out(cmd), req, params, now)
		},
	}
}

// sample reads the --payload, --header, --param and --now flags of `preset test`.
func sample(cmd *cli.Command) (preset.Request, preset.Params, time.Time, error) {
	body, err := os.ReadFile(cmd.String("payload"))
	if err != nil {
		return preset.Request{}, nil, time.Time{}, err
	}
	headers, err := pairs(cmd.StringSlice("header"))
	if err != nil {
		return preset.Request{}, nil, time.Time{}, fmt.Errorf("--header: %w", err)
	}
	header := http.Header{}
	for k, v := range headers {
		header.Set(k, v)
	}
	values, err := pairs(cmd.StringSlice("param"))
	if err != nil {
		return preset.Request{}, nil, time.Time{}, fmt.Errorf("--param: %w", err)
	}
	params := preset.Params{}
	for k, v := range values {
		params[k] = v
	}
	now := time.Now()
	if raw := cmd.String("now"); raw != "" {
		if now, err = time.Parse(time.RFC3339, raw); err != nil {
			return preset.Request{}, nil, time.Time{}, fmt.Errorf("--now is not an RFC 3339 time: %w", err)
		}
	}
	return preset.Request{Header: header, Body: body}, params, now, nil
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

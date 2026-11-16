package config

import (
	"fmt"
	"strings"

	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
)

// Mode is the kind of deployment an Instance is (ADR 0060). It alone decides what differs
// between a self-hosted Instance and the SaaS; no separate setting does.
type Mode string

const (
	// ModeSelfHosted is the default: one company's Instance, run by its Self-hoster.
	ModeSelfHosted Mode = "selfhosted"
	// ModeSaaS is the Instance the platform runs for many customers.
	ModeSaaS Mode = "saas"
)

// UnmarshalText reads SPHERICON_MODE, in any case: anything but a known value is an error, so a
// typo never starts an Instance in the wrong Mode.
func (m *Mode) UnmarshalText(text []byte) error {
	switch v := Mode(strings.ToLower(string(text))); v {
	case ModeSelfHosted, ModeSaaS:
		*m = v
		return nil
	default:
		return fmt.Errorf("SPHERICON_MODE must be %q or %q, got %q", ModeSelfHosted, ModeSaaS, text)
	}
}

// SaaS reports whether the Mode is saas.
func (m Mode) SaaS() bool { return m == ModeSaaS }

// ApplyMode sets the Mode and every setting it decides (ADR 0060). Load calls it; a test calls it
// to run a built Config in the other Mode.
func (c *Config) ApplyMode(m Mode) {
	c.Mode = m
	c.ReleaseCheck = !m.SaaS()
	c.RequireEmailVerification = m.SaaS()
	c.SignupSurvey = m.SaaS()
	c.Telemetry = nil
	if m.SaaS() {
		// The saas Mode sends no Telemetry report and locks the setting off (ADR 0032).
		off := false
		c.Telemetry = &off
	}
}

// TrustsDevLicense is whether a license key signed by the development key is accepted (ADR 0060):
// in a development or end-to-end env, and never in a release build: SPHERICON_ENV alone would let a
// self-hoster who sets it to development unlock `ee/`, and the build's version, which only a
// release carries, closes that.
func (c *Config) TrustsDevLicense() bool {
	return (c.IsDev || c.IsE2E) && !buildinfo.IsRelease(buildinfo.Current().Version)
}

// MigratesFromThePage is whether the server starts on a database that has not run its migrations
// and lets the error page run them (ADR 0033): a development or test env, never a release build,
// for the reason TrustsDevLicense gives. Such a server answers 503 until the migrations run.
func (c *Config) MigratesFromThePage() bool {
	return c.IsDev && !buildinfo.IsRelease(buildinfo.Current().Version)
}

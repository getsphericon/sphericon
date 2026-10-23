package signupsurvey_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/signupsurvey"
)

func TestTheSurveyIsOnOnlyWithTheSwitchAndTheLicense(t *testing.T) {
	cases := map[string]struct {
		requested, licensed bool
		on                  bool
		warnings            int
	}{
		"switch and license":     {requested: true, licensed: true, on: true, warnings: 0},
		"switch without license": {requested: true, licensed: false, on: false, warnings: 1},
		"license without switch": {requested: false, licensed: true, on: false, warnings: 0},
		"neither":                {requested: false, licensed: false, on: false, warnings: 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			log := slog.New(slog.NewTextHandler(&out, nil))

			on := signupsurvey.Enabled(c.requested, c.licensed, log)

			assert.Equal(t, c.on, on)
			assert.Equal(t, c.warnings, strings.Count(out.String(), "level=WARN"))
		})
	}
}

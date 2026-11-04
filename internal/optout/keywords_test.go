package optout_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/optout"
)

func TestIsStopKeyword(t *testing.T) {
	cases := []struct {
		text string
		stop bool
	}{
		{"STOP", true},
		{"stop", true},
		{"Stop", true},
		{"  stop\n", true},
		{"STOPALL", true},
		{"UNSUBSCRIBE", true},
		{"cancel", true},
		{"End", true},
		{"QUIT", true},
		{"СТОП", true},
		{"стоп", true},
		{"Отписаться", true},
		{"stop it", false},
		{"please do not stop", false},
		{"stop!", false},
		{"stopx", false},
		{"/stop", false},
		{"", false},
		{"   ", false},
		{"start", false},
		{"unsubscribed", false},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			assert.Equal(t, c.stop, optout.IsStopKeyword(c.text))
		})
	}
}

func TestIsStartKeyword(t *testing.T) {
	cases := []struct {
		text  string
		start bool
	}{
		{"START", true},
		{"start", true},
		{" Start\n", true},
		{"UNSTOP", true},
		{"unstop", true},
		{"start again", false},
		{"please start", false},
		{"/start", false},
		{"stop", false},
		{"", false},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			assert.Equal(t, c.start, optout.IsStartKeyword(c.text))
		})
	}
}

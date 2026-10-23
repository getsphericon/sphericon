package server

import (
	_ "embed"
	"net/http"
)

// trackerScript is the standalone browser snippet built from packages/analytics
// (the IIFE bundle). `make build-tracker` regenerates and copies it here so the
// single binary serves it from /t.js with no extra infrastructure (self-host).
//
//go:embed assets/t.js
var trackerScript []byte

func trackerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(trackerScript)
	})
}

// Package export is the Enterprise side of the Export streams (ADR 0045): the `export`
// license gate core asks before it serves a stream. The paging, the cursor and the row
// encoding are core (internal/exportstream); the streams that need Enterprise state
// (the Erasures tombstone subscriber, its prune job) live here.
//
// Governed by ee/LICENSE, not the AGPL.
package export

import "github.com/getsphericon/sphericon/ee/licensekey"

// Streams is the Edition's seat of the Export streams.
type Streams struct {
	lic *licensekey.License
}

// New builds the Export streams for a license.
func New(lic *licensekey.License) *Streams { return &Streams{lic: lic} }

// Licensed reports whether the `export` feature is licensed; unlicensed, every stream
// answers 404 like the /staff surface (ADR 0045).
func (s *Streams) Licensed() bool { return s != nil && s.lic.Has(licensekey.FeatureExport) }

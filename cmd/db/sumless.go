package main

import (
	"io/fs"
	"strings"
	"time"

	"ariga.io/atlas/sql/migrate"
)

// sumlessDir hides Atlas's integrity file from the library. ent's diff engine insists on a
// valid atlas.sum next to the migrations (it refuses a directory that has files and no sum)
// and writes a fresh one after every migration it plans. Goose does not use the file, so
// this wrapper serves a sum computed from the directory on the fly and drops the write:
// nothing is stored and nothing can go stale.
type sumlessDir struct{ migrate.Dir }

func (d sumlessDir) Open(name string) (fs.File, error) {
	if name != migrate.HashFileName {
		return d.Dir.Open(name)
	}
	sum, err := d.Checksum()
	if err != nil {
		return nil, err
	}
	text, err := sum.MarshalText()
	if err != nil {
		return nil, err
	}
	return memFile{Reader: strings.NewReader(string(text)), name: name, size: int64(len(text))}, nil
}

func (d sumlessDir) WriteFile(name string, b []byte) error {
	if name == migrate.HashFileName {
		return nil
	}
	return d.Dir.WriteFile(name, b)
}

// memFile is a read-only fs.File over bytes held in memory.
type memFile struct {
	*strings.Reader
	name string
	size int64
}

func (f memFile) Stat() (fs.FileInfo, error) { return f, nil }
func (memFile) Close() error                 { return nil }
func (f memFile) Name() string               { return f.name }
func (f memFile) Size() int64                { return f.size }
func (memFile) Mode() fs.FileMode            { return 0o444 }
func (memFile) ModTime() time.Time           { return time.Time{} }
func (memFile) IsDir() bool                  { return false }
func (memFile) Sys() any                     { return nil }

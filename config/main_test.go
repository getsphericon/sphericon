package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// committedEnvFiles are the env files the repository ships that Load reads (.env.sample is a
// template, never read). The tests load these and nothing else: a developer's own .env,
// .env.local and .env.<env>.local are gitignored and would fill in what a test leaves empty.
var committedEnvFiles = []string{".env.test"}

func TestMain(m *testing.M) {
	dir := mustCommittedEnvRoot()
	envRoot = func() (string, error) { return dir, nil }
	code := m.Run()
	cleanup.Report("remove the env files' directory", os.RemoveAll(dir))
	os.Exit(code)
}

// mustCommittedEnvRoot is committedEnvRoot; a checkout it cannot copy from stops the package.
func mustCommittedEnvRoot() string {
	dir, err := committedEnvRoot()
	if err != nil {
		panic(fmt.Sprintf("config tests: %v", err))
	}
	return dir
}

// committedEnvRoot copies committedEnvFiles from the repository root into a fresh directory.
func committedEnvRoot() (string, error) {
	root, err := envRoot()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "sphericon-config-env")
	if err != nil {
		return "", err
	}
	for _, name := range committedEnvFiles {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

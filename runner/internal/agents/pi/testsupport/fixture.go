// Package testsupport provides testing-only fixture helpers for the pi agent.
// Kept separate so the production pi library does not embed fixture bytes.
package testsupport

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

//go:embed testdata/pi_session.jsonl
var fixtureSession []byte

// BuildFixtureSandbox plants the pi session fixture in the pod-local layout the
// pi parser scans (`<sandbox>/pi-home/sessions/<cwd-hash>/`) and returns the
// sandbox path for parser.Parse(...).
func BuildFixtureSandbox(t *testing.T) string {
	return buildFixtureSandbox(t, "pi-home")
}

// BuildLeanFixtureSandbox plants the fixture under the lean wrapper's profile
// dir (`<sandbox>/pi-lean-home/sessions/<cwd-hash>/`), the layout pi-pod-lean /
// pi-lean-cli produce via PI_POD_LEAN_PROFILE_DIR.
func BuildLeanFixtureSandbox(t *testing.T) string {
	return buildFixtureSandbox(t, "pi-lean-home")
}

func buildFixtureSandbox(t *testing.T, home string) string {
	t.Helper()
	sandbox := t.TempDir()
	dir := filepath.Join(sandbox, home, "sessions", "--fixture--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("pi fixture: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), fixtureSession, 0o644); err != nil {
		t.Fatalf("pi fixture: write: %v", err)
	}
	return sandbox
}

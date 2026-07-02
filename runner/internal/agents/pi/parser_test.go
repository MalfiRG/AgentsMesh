package pi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anthropics/agentsmesh/runner/internal/agents/pi/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPiParser_SumsAssistantUsageByModel(t *testing.T) {
	sandbox := testsupport.BuildFixtureSandbox(t)

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.False(t, usage.IsEmpty())

	m := usage.Models["gpt-5.5"]
	require.NotNil(t, m, "expected gpt-5.5 attribution, got %v", usage.Models)
	assert.Equal(t, int64(17341+2048), m.InputTokens)
	assert.Equal(t, int64(177+512), m.OutputTokens)
	assert.Equal(t, int64(25600), m.CacheReadTokens)
	assert.Equal(t, int64(1024), m.CacheCreationTokens)
}

func TestPiParser_ScansLeanProfileDir(t *testing.T) {
	sandbox := testsupport.BuildLeanFixtureSandbox(t)

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	require.NotNil(t, usage, "lean wrapper sessions under pi-lean-home must be counted")
	require.False(t, usage.IsEmpty())
	assert.NotNil(t, usage.Models["gpt-5.5"])
}

func TestPiParser_IgnoresSymlinkedSessionFile(t *testing.T) {
	sandbox := testsupport.BuildLeanFixtureSandbox(t)
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	require.NoError(t, os.WriteFile(outside,
		[]byte(`{"type":"message","message":{"model":"gpt-9","usage":{"input":999}}}`), 0o644))
	link := filepath.Join(sandbox, "pi-lean-home", "sessions", "--fixture--", "link.jsonl")
	require.NoError(t, os.Symlink(outside, link))

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Nil(t, usage.Models["gpt-9"], "symlinked session file must not be parsed")
}

func TestPiParser_NoSessions_ReturnsNil(t *testing.T) {
	usage, err := (&piParser{}).Parse(t.TempDir(), time.Unix(0, 0))
	require.NoError(t, err)
	assert.Nil(t, usage)
}

package pi

import (
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

func TestPiParser_NoSessions_ReturnsNil(t *testing.T) {
	usage, err := (&piParser{}).Parse(t.TempDir(), time.Unix(0, 0))
	require.NoError(t, err)
	assert.Nil(t, usage)
}

package runner

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/agentsmesh/runner/internal/relay"
)

type captureRelay struct {
	events [][]byte
}

func (c *captureRelay) BroadcastEvent(_ relay.RelayClient, _ byte, payload []byte) {
	c.events = append(c.events, payload)
}
func (c *captureRelay) SetupHandlers(relay.RelayClient)    {}
func (c *captureRelay) SendSnapshot(relay.RelayClient)     {}
func (c *captureRelay) OnRelayConnected(relay.RelayClient) {}
func (c *captureRelay) OnRelayDisconnected()               {}

// An ACP-mode submit echoes the user message as a "contentChunk" event — the
// camelCase form the ACP dispatcher matches. Regression for the snake_case
// "content_chunk" bug where the echo silently never rendered.
func TestSubmitPromptToPod_ACP_EchoesContentChunk(t *testing.T) {
	rel := &captureRelay{}
	pod := &Pod{
		PodKey:          "acp-pod",
		InteractionMode: InteractionModeACP,
		IO:              &sendPromptMockIO{mode: InteractionModeACP},
		Relay:           rel,
	}

	if err := submitPromptToPod(pod, "hi"); err != nil {
		t.Fatalf("submitPromptToPod: %v", err)
	}

	if len(rel.events) != 1 {
		t.Fatalf("expected one relay echo; got %d", len(rel.events))
	}
	var ev map[string]any
	if err := json.Unmarshal(rel.events[0], &ev); err != nil {
		t.Fatalf("echo payload not JSON: %v", err)
	}
	if ev["type"] != "contentChunk" {
		t.Errorf("echo type = %v, want contentChunk", ev["type"])
	}
	if ev["role"] != "user" || ev["text"] != "hi" {
		t.Errorf("echo role/text = %v/%v, want user/hi", ev["role"], ev["text"])
	}
}

package runner

import (
	"fmt"
	"strings"
	"time"

	runnerv1 "github.com/anthropics/agentsmesh/proto/gen/go/runner/v1"
	"github.com/anthropics/agentsmesh/runner/internal/client"
	"github.com/anthropics/agentsmesh/runner/internal/logger"
)

// ptySubmitGap separates the prompt-body Write from the Enter keystroke so
// the TUI's read(2) loop ticks between them. Without this gap both writes
// land in one read and the TUI treats the whole chunk (incl. trailing \r)
// as a paste — the Enter never fires. MCP's two-RPC path gets this gap
// implicitly via network round-trip; the in-process gRPC path does not.
const ptySubmitGap = 80 * time.Millisecond

// OnListRelayConnections returns current relay connections.
func (h *RunnerMessageHandler) OnListRelayConnections() []client.RelayConnectionInfo {
	pods := h.podStore.All()
	result := make([]client.RelayConnectionInfo, 0)

	for _, pod := range pods {
		relayClient := pod.GetRelayClient()
		if relayClient != nil {
			result = append(result, client.RelayConnectionInfo{
				PodKey:      pod.PodKey,
				RelayURL:    relayClient.GetRelayURL(),
				Connected:   relayClient.IsConnected(),
				ConnectedAt: relayClient.GetConnectedAt(),
			})
		}
	}

	return result
}

// OnPodInput handles PTY input from server.
func (h *RunnerMessageHandler) OnPodInput(req client.PodInputRequest) error {
	log := logger.Pod()
	pod, ok := h.podStore.Get(req.PodKey)
	if !ok {
		log.Warn("Pod not found for PTY input", "pod_key", req.PodKey)
		return fmt.Errorf("pod not found: %s", req.PodKey)
	}
	if pod.IO == nil {
		log.Warn("PodIO not available for input", "pod_key", req.PodKey)
		return fmt.Errorf("pod IO not available for pod: %s", req.PodKey)
	}
	if err := pod.IO.SendInput(string(req.Data)); err != nil {
		log.Error("Failed to write pod input", "pod_key", req.PodKey, "error", err)
		return err
	}
	return nil
}

// OnQuerySandboxes handles sandbox status query from server.
func (h *RunnerMessageHandler) OnQuerySandboxes(req client.QuerySandboxesRequest) error {
	log := logger.Pod()
	log.Info("Querying sandbox status", "request_id", req.RequestID, "queries", len(req.Queries))

	results := make([]*client.SandboxStatusInfo, 0, len(req.Queries))
	for _, query := range req.Queries {
		status := h.runner.GetSandboxStatus(query.PodKey)
		results = append(results, status)
	}

	if err := h.conn.SendSandboxesStatus(req.RequestID, results); err != nil {
		log.Error("Failed to send sandbox status response", "request_id", req.RequestID, "error", err)
		return err
	}

	log.Info("Sent sandbox status response", "request_id", req.RequestID, "results", len(results))
	return nil
}

// OnObservePod handles observe PTY command from server.
// Reads pod I/O state and sends result back via gRPC.
func (h *RunnerMessageHandler) OnObservePod(req client.ObservePodRequest) error {
	log := logger.Pod()

	pod, ok := h.podStore.Get(req.PodKey)
	if !ok {
		log.Warn("Pod not found for observe PTY", "pod_key", req.PodKey)
		return h.conn.SendObservePodResult(req.RequestID, req.PodKey, "", "", 0, 0, 0, false, "pod not found")
	}

	if pod.IO == nil {
		log.Warn("No PodIO for observe PTY", "pod_key", req.PodKey)
		return h.conn.SendObservePodResult(req.RequestID, req.PodKey, "", "", 0, 0, 0, false, "pod IO not available")
	}

	lines := req.Lines
	if lines <= 0 {
		lines = 100
	}

	output, err := pod.IO.GetSnapshot(lines)
	if err != nil {
		log.Error("Failed to get snapshot for observe PTY", "pod_key", req.PodKey, "error", err)
		return h.conn.SendObservePodResult(req.RequestID, req.PodKey, "", "", 0, 0, 0, false, err.Error())
	}
	var cursorY, cursorX int
	var screen string
	if ta, ok := pod.IO.(TerminalAccess); ok {
		cursorY, cursorX = ta.CursorPosition()
		if req.IncludeScreen {
			screen = ta.GetScreenSnapshot()
		}
	}

	// Count total lines in output to determine hasMore
	totalLines := 0
	if output != "" {
		totalLines = strings.Count(output, "\n") + 1
	}
	hasMore := totalLines >= lines

	if err := h.conn.SendObservePodResult(req.RequestID, req.PodKey, output, screen, cursorX, cursorY, totalLines, hasMore, ""); err != nil {
		log.Error("Failed to send observe PTY result", "request_id", req.RequestID, "error", err)
		return err
	}

	log.Debug("Sent observe PTY result", "request_id", req.RequestID, "pod_key", req.PodKey, "lines", totalLines)
	return nil
}

// OnSendPrompt handles the send_prompt command from the server (gRPC control
// plane), delegating submission to submitPromptToPod.
func (h *RunnerMessageHandler) OnSendPrompt(cmd *runnerv1.SendPromptCommand) error {
	log := logger.Pod()
	pod, ok := h.podStore.Get(cmd.PodKey)
	if !ok {
		log.Warn("Pod not found for send_prompt", "pod_key", cmd.PodKey)
		return fmt.Errorf("pod not found: %s", cmd.PodKey)
	}
	if pod.IO == nil {
		log.Warn("PodIO not available for send_prompt", "pod_key", cmd.PodKey)
		return fmt.Errorf("pod IO not available: %s", cmd.PodKey)
	}
	return submitPromptToPod(pod, cmd.Prompt)
}

// submitPromptToPod is the shared prompt-submit path for the gRPC control plane
// (OnSendPrompt) and the cross-pod MCP path (Runner.SendPodInput).
func submitPromptToPod(pod *Pod, prompt string) error {
	if pod.IsACPMode() {
		sendAcpViaRelay(pod, "contentChunk", "", map[string]string{
			"text": prompt, "role": "user",
		})
	}
	if err := pod.IO.SendInput(prompt); err != nil {
		return err
	}
	if ta, ok := pod.IO.(TerminalAccess); ok {
		time.Sleep(ptySubmitGap)
		return ta.SendKeys([]string{"enter"})
	}
	return nil
}

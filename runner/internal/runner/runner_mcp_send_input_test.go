package runner

import (
	"errors"
	"testing"
	"time"

	"github.com/anthropics/agentsmesh/runner/internal/mcp"
)

func TestSendPodInput_Text_SubmitsViaEnter(t *testing.T) {
	base := &sendPromptMockIO{mode: InteractionModePTY}
	io := &ptyTerminalMock{sendPromptMockIO: base}
	pod := &Pod{PodKey: "target-pod", InteractionMode: InteractionModePTY, IO: io}

	store := NewInMemoryPodStore()
	store.Put(pod.PodKey, pod)
	r := &Runner{podStore: store}

	if err := r.SendPodInput(pod.PodKey, "hello from A", nil); err != nil {
		t.Fatalf("SendPodInput error: %v", err)
	}

	base.mu.Lock()
	defer base.mu.Unlock()
	if len(base.inputs) != 1 || base.inputs[0].payload != "hello from A" {
		t.Fatalf("expected 1 SendInput with the body; got %v", base.inputs)
	}
	if len(base.keys) != 1 || base.keys[0].payload != "enter" {
		t.Fatalf("expected one SendKeys([\"enter\"]) to submit the prompt; got %v", base.keys)
	}
	if gap := base.keys[0].at.Sub(base.inputs[0].at); gap < 50*time.Millisecond {
		t.Fatalf("gap between body and Enter = %v, want >= 50ms (TUI read-loop separation)", gap)
	}
}

func TestSendPodInput_KeysOnly_NoImplicitEnter(t *testing.T) {
	base := &sendPromptMockIO{mode: InteractionModePTY}
	io := &ptyTerminalMock{sendPromptMockIO: base}
	pod := &Pod{PodKey: "target-pod", InteractionMode: InteractionModePTY, IO: io}

	store := NewInMemoryPodStore()
	store.Put(pod.PodKey, pod)
	r := &Runner{podStore: store}

	if err := r.SendPodInput(pod.PodKey, "", []string{"ctrl+c"}); err != nil {
		t.Fatalf("SendPodInput error: %v", err)
	}

	base.mu.Lock()
	defer base.mu.Unlock()
	if len(base.inputs) != 0 {
		t.Fatalf("keys-only must not call SendInput; got %v", base.inputs)
	}
	if len(base.keys) != 1 || base.keys[0].payload != "ctrl+c" {
		t.Fatalf("expected the raw key only; got %v", base.keys)
	}
}

func TestSendPodInput_UnknownPod_ReturnsErrPodNotLocal(t *testing.T) {
	r := &Runner{podStore: NewInMemoryPodStore()}
	err := r.SendPodInput("ghost", "hi", nil)
	if !errors.Is(err, mcp.ErrPodNotLocal) {
		t.Fatalf("want ErrPodNotLocal for unknown pod, got %v", err)
	}
}

func TestSendPodInput_LocalFailure_NotErrPodNotLocal(t *testing.T) {
	base := &sendPromptMockIO{mode: InteractionModeACP}
	pod := &Pod{PodKey: "local-pod", InteractionMode: InteractionModeACP, IO: base}
	store := NewInMemoryPodStore()
	store.Put(pod.PodKey, pod)
	r := &Runner{podStore: store}

	err := r.SendPodInput("local-pod", "hi", []string{"ctrl+c"})
	if err == nil {
		t.Fatal("expected an error: keys unsupported by non-TerminalAccess IO")
	}
	if errors.Is(err, mcp.ErrPodNotLocal) {
		t.Fatalf("local send failure must not be ErrPodNotLocal, got %v", err)
	}
}

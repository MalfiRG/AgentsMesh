package grpc

import (
	"context"
	"testing"

	channelDomain "github.com/anthropics/agentsmesh/backend/internal/domain/channel"
)

// The self-access and unknown-caller branches short-circuit before touching
// bindingService, so a bare *GRPCRunnerAdapter (nil deps) exercises them. The
// HasScope path is covered by binding_integration_test.go.

func TestRequirePodBinding_SelfAccessAllowed(t *testing.T) {
	a := &GRPCRunnerAdapter{}
	if mcpErr := a.requirePodBinding(context.Background(), "pod-x", "pod-x", channelDomain.BindingScopePodWrite); mcpErr != nil {
		t.Fatalf("a pod acting on itself should be allowed, got %v", mcpErr)
	}
}

func TestRequirePodBinding_UnknownCallerDenied(t *testing.T) {
	a := &GRPCRunnerAdapter{}
	mcpErr := a.requirePodBinding(context.Background(), "", "pod-y", channelDomain.BindingScopePodRead)
	if mcpErr == nil || mcpErr.code != 403 {
		t.Fatalf("expected 403 for unknown caller pod, got %v", mcpErr)
	}
}

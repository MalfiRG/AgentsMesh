package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/agentsmesh/backend/internal/service/runner"
)

type routeCall struct {
	pod  string
	data string
}

type fakeRouteRouter struct {
	prompts   []routeCall
	inputs    []routeCall
	promptErr error
	inputErr  error
}

func (f *fakeRouteRouter) RoutePrompt(podKey, prompt string) error {
	f.prompts = append(f.prompts, routeCall{podKey, prompt})
	return f.promptErr
}

func (f *fakeRouteRouter) RoutePodInput(podKey string, data []byte) error {
	f.inputs = append(f.inputs, routeCall{podKey, string(data)})
	return f.inputErr
}

func (f *fakeRouteRouter) ObservePod(context.Context, string, int32, bool) (*runner.ObservePodResult, error) {
	return nil, nil
}

func TestRouteSendPodInput_TextOnly_UsesRoutePrompt(t *testing.T) {
	f := &fakeRouteRouter{}
	if e := routeSendPodInput(f, "pod", "hello", nil); e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	if len(f.prompts) != 1 || f.prompts[0].data != "hello" {
		t.Fatalf("want one RoutePrompt(hello); got %v", f.prompts)
	}
	if len(f.inputs) != 0 {
		t.Fatalf("text must not go through RoutePodInput; got %v", f.inputs)
	}
}

func TestRouteSendPodInput_TextWithEnter_SkipsRedundantEnter(t *testing.T) {
	f := &fakeRouteRouter{}
	if e := routeSendPodInput(f, "pod", "hello", []string{"enter"}); e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	if len(f.prompts) != 1 {
		t.Fatalf("want one RoutePrompt; got %v", f.prompts)
	}
	if len(f.inputs) != 0 {
		t.Fatalf("redundant enter must be skipped; got %v", f.inputs)
	}
}

func TestRouteSendPodInput_TextWithMixedKeys_SkipsOnlyEnter(t *testing.T) {
	f := &fakeRouteRouter{}
	if e := routeSendPodInput(f, "pod", "hello", []string{"ctrl+c", "enter", "escape"}); e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	if len(f.inputs) != 2 {
		t.Fatalf("want ctrl+c and escape only (enter skipped); got %v", f.inputs)
	}
	if f.inputs[0].data != "\x03" || f.inputs[1].data != "\x1b" {
		t.Fatalf("want [ctrl+c, escape] raw bytes; got %q, %q", f.inputs[0].data, f.inputs[1].data)
	}
}

func TestRouteSendPodInput_KeysOnly_RawNoPrompt(t *testing.T) {
	f := &fakeRouteRouter{}
	if e := routeSendPodInput(f, "pod", "", []string{"enter"}); e != nil {
		t.Fatalf("unexpected error: %v", e)
	}
	if len(f.prompts) != 0 {
		t.Fatalf("keys-only must not call RoutePrompt; got %v", f.prompts)
	}
	if len(f.inputs) != 1 || f.inputs[0].data != "\r" {
		t.Fatalf("want raw enter (\\r); got %v", f.inputs)
	}
}

func TestRouteSendPodInput_RoutePromptError_Returns500(t *testing.T) {
	f := &fakeRouteRouter{promptErr: errors.New("boom")}
	e := routeSendPodInput(f, "pod", "hello", nil)
	if e == nil || e.code != 500 {
		t.Fatalf("want 500 mcpError; got %v", e)
	}
	if len(f.inputs) != 0 {
		t.Fatalf("must not send keys after a prompt failure; got %v", f.inputs)
	}
}

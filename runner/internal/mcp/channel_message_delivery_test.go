package mcp

import "testing"

func TestAnnotateChannelDelivery_NoMentionsAttachesNotice(t *testing.T) {
	msg := map[string]interface{}{"id": 7}

	out := annotateChannelDelivery(msg, nil)

	wrapped, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("expected wrapped map, got %T", out)
	}
	if wrapped["delivery_notice"] != passiveSendNotice {
		t.Errorf("expected passive notice, got %v", wrapped["delivery_notice"])
	}
	if wrapped["message"] == nil {
		t.Error("expected original message preserved under \"message\"")
	}
}

func TestNormalizePodMentions(t *testing.T) {
	got := normalizePodMentions([]string{"3-standalone-85c8e6bd", "pod:already", "user:5"})
	want := []string{"pod:3-standalone-85c8e6bd", "pod:already", "user:5"}
	if len(got) != len(want) {
		t.Fatalf("len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q want %q", i, got[i], want[i])
		}
	}
	if normalizePodMentions(nil) != nil {
		t.Error("nil in must return nil out")
	}
}

func TestAnnotateChannelDelivery_WithMentionsReturnsRaw(t *testing.T) {
	msg := map[string]interface{}{"id": 7}

	out := annotateChannelDelivery(msg, []string{"3-134-02fb30bf"})

	if _, wrapped := out.(map[string]interface{})["delivery_notice"]; wrapped {
		t.Error("mentioned send must not carry a passive delivery notice")
	}
}

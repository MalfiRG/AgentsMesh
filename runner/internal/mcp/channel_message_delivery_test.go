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

func TestAnnotateChannelDelivery_WithMentionsReturnsRaw(t *testing.T) {
	msg := map[string]interface{}{"id": 7}

	out := annotateChannelDelivery(msg, []string{"3-134-02fb30bf"})

	if _, wrapped := out.(map[string]interface{})["delivery_notice"]; wrapped {
		t.Error("mentioned send must not carry a passive delivery notice")
	}
}

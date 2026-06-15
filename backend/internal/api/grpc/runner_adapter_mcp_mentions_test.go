package grpc

import (
	"testing"

	channelDomain "github.com/anthropics/agentsmesh/backend/internal/domain/channel"
)

func mentionKeys(c channelDomain.MessageContent) []string {
	var out []string
	for _, b := range c.Blocks {
		for _, e := range b.Elements {
			if e.Type == channelDomain.InlineMention {
				out = append(out, e.EntityType+":"+e.EntityKey)
			}
		}
	}
	return out
}

func TestEnsureParamMentionsEmbedded_AddsMissing(t *testing.T) {
	content := channelDomain.MessageContent{Kind: "text", Blocks: []channelDomain.Block{
		{Type: "paragraph", Elements: []channelDomain.InlineElement{{Type: channelDomain.InlineText, Text: "no at-token here"}}},
	}}
	mentions := map[string]struct{ typ, key string }{"3-x-aa": {"pod", "3-x-aa"}}

	got := mentionKeys(ensureParamMentionsEmbedded(content, mentions))
	if len(got) != 1 || got[0] != "pod:3-x-aa" {
		t.Fatalf("expected [pod:3-x-aa], got %v", got)
	}
}

func TestEnsureParamMentionsEmbedded_NoDuplicate(t *testing.T) {
	content := channelDomain.MessageContent{Kind: "text", Blocks: []channelDomain.Block{
		{Type: "paragraph", Elements: []channelDomain.InlineElement{
			{Type: channelDomain.InlineMention, EntityType: "pod", EntityKey: "3-x-aa", Display: "3-x-aa"},
		}},
	}}
	mentions := map[string]struct{ typ, key string }{"3-x-aa": {"pod", "3-x-aa"}}

	if got := mentionKeys(ensureParamMentionsEmbedded(content, mentions)); len(got) != 1 {
		t.Fatalf("expected no duplicate, got %v", got)
	}
}

func TestEnsureParamMentionsEmbedded_EmptyNoop(t *testing.T) {
	content := channelDomain.MessageContent{Kind: "text", Blocks: []channelDomain.Block{
		{Type: "paragraph", Elements: []channelDomain.InlineElement{{Type: channelDomain.InlineText, Text: "x"}}},
	}}
	if got := mentionKeys(ensureParamMentionsEmbedded(content, nil)); got != nil {
		t.Fatalf("expected no mentions, got %v", got)
	}
}

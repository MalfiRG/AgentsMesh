package grpc

import (
	"sort"

	channelDomain "github.com/anthropics/agentsmesh/backend/internal/domain/channel"
)

// ensureParamMentionsEmbedded guarantees every mention passed in the explicit
// MCP `mentions` param is present as an InlineMention in the content. The
// channel service derives msg.Mentions from content alone, so without this a
// pod-originated mention that lacks a literal @<key> token in the body is
// dropped and PodPromptHook never pushes to the target pod.
func ensureParamMentionsEmbedded(content channelDomain.MessageContent, mentions map[string]struct{ typ, key string }) channelDomain.MessageContent {
	if len(mentions) == 0 {
		return content
	}

	embedded := make(map[string]bool)
	for _, b := range content.Blocks {
		for _, e := range b.Elements {
			if e.Type == channelDomain.InlineMention {
				embedded[e.EntityType+":"+e.EntityKey] = true
			}
		}
	}

	keys := make([]string, 0, len(mentions))
	for k := range mentions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var missing []channelDomain.InlineElement
	for _, k := range keys {
		m := mentions[k]
		if !embedded[m.typ+":"+m.key] {
			missing = append(missing, channelDomain.InlineElement{
				Type: channelDomain.InlineMention, EntityType: m.typ, EntityKey: m.key, Display: m.key,
			})
		}
	}
	if len(missing) == 0 {
		return content
	}

	content.Blocks = append(content.Blocks, channelDomain.Block{Type: "paragraph", Elements: missing})
	return content
}

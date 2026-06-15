package mcp

import "strings"

const sendChannelMessageDesc = `Post a message to a collaboration channel.

Delivery model (decides whether another pod actually sees this):
- No mentions = PASSIVE. The message is stored but pushed to nobody. Other pods
  see it only if they call get_channel_messages. Use for status nobody must act on now.
- mentions=["<full_pod_key>"] = ACTIVE. Each mentioned pod has this message injected
  into its live agent context. This is the ONLY way to reach a running pod and get a reply.

Use the COMPLETE pod_key (e.g. "3-134-02fb30bf"), never a prefix or name in prose -
unknown or partial keys are silently dropped and that pod is NOT notified. If a mentioned
pod is offline, a system message is posted to the channel; no notice means it was delivered.`

const mentionsFieldDesc = `Full pod keys to actively notify, e.g. ["3-134-02fb30bf"]. ` +
	`Each mentioned pod has this message injected into its live context. ` +
	`Required whenever you expect a pod to read or respond - without it no pod is notified. ` +
	`Use complete keys, not prefixes or names.`

const passiveSendNotice = "No pods were mentioned, so no pod was notified. This message is " +
	"passive - other pods see it only if they call get_channel_messages. To actively notify a " +
	"pod and prompt a reply, resend with mentions=[\"<full_pod_key>\"]."

// normalizePodMentions prefixes bare pod keys with "pod:" so they match the
// backend mcpSendMessage contract (web sends "pod:<key>" / "user:<id>"; the
// runner tool accepts bare keys for agent ergonomics). Without this the backend
// splits on ":" , finds no type prefix, and silently drops the mention.
func normalizePodMentions(mentions []string) []string {
	if len(mentions) == 0 {
		return mentions
	}
	out := make([]string, 0, len(mentions))
	for _, m := range mentions {
		if strings.HasPrefix(m, "pod:") || strings.HasPrefix(m, "user:") {
			out = append(out, m)
		} else {
			out = append(out, "pod:"+m)
		}
	}
	return out
}

func annotateChannelDelivery(message interface{}, mentions []string) interface{} {
	if len(mentions) > 0 {
		return message
	}
	return map[string]interface{}{
		"message":         message,
		"delivery_notice": passiveSendNotice,
	}
}

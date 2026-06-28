package runner

import (
	"strconv"

	"github.com/anthropics/agentsmesh/runner/internal/logger"
	"github.com/anthropics/agentsmesh/runner/internal/terminal/vt"
)

// isConEmuControlSubcommand reports whether an OSC 9 sequence is the ConEmu
// control sub-protocol (`9;4;...` progress, `9;9;...` cwd, etc.) rather than a
// plain `9;<text>` toast. Progress ticks would otherwise be misread as toasts.
func isConEmuControlSubcommand(params []string) bool {
	if len(params) == 0 {
		return false
	}
	if _, err := strconv.Atoi(params[0]); err != nil {
		return false
	}
	return params[0] == "4" || len(params) >= 2
}

// createOSCHandler creates an OSC handler that sends terminal notifications to the server.
func (h *RunnerMessageHandler) createOSCHandler(podKey string) vt.OSCHandler {
	return func(oscType int, params []string) {
		log := logger.TerminalTrace()

		switch oscType {
		case 777:
			// OSC 777;notify;title;body - iTerm2/Kitty notification format
			if len(params) >= 3 && params[0] == "notify" {
				title := params[1]
				body := params[2]
				log.Trace("OSC 777 notification detected", "pod_key", podKey, "title", title, "body", body)
				if err := h.conn.SendOSCNotification(podKey, title, body); err != nil {
					log.Error("Failed to send OSC notification", "pod_key", podKey, "error", err)
				}
			}

		case 9:
			// OSC 9 is overloaded: `9;<text>` is a ConEmu/Windows-Terminal
			// toast, but `9;4;state;pct` (and other `9;<digit>;...` forms)
			// is the ConEmu progress/control sub-protocol. Progress bars
			// emit `9;4;...` on every tick; forwarding those as toasts spams
			// "Notification: 4" without end. Only `9;<text>` is a real toast.
			if len(params) >= 1 && !isConEmuControlSubcommand(params) {
				body := params[0]
				log.Trace("OSC 9 notification detected", "pod_key", podKey, "body", body)
				if err := h.conn.SendOSCNotification(podKey, "Notification", body); err != nil {
					log.Error("Failed to send OSC notification", "pod_key", podKey, "error", err)
				}
			}

		case 0, 2:
			// OSC 0/2;title - Window/tab title
			if len(params) >= 1 {
				title := params[0]
				log.Trace("OSC title change detected", "pod_key", podKey, "title", title)
				if err := h.conn.SendOSCTitle(podKey, title); err != nil {
					log.Error("Failed to send OSC title", "pod_key", podKey, "error", err)
				}
			}
		}
	}
}

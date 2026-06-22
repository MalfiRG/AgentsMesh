package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/anthropics/agentsmesh/backend/internal/domain/agentpod"
	notifDomain "github.com/anthropics/agentsmesh/backend/internal/domain/notification"
	"github.com/anthropics/agentsmesh/backend/internal/infra/eventbus"
	notifService "github.com/anthropics/agentsmesh/backend/internal/service/notification"
	"github.com/anthropics/agentsmesh/backend/internal/service/runner"
	eventsv1 "github.com/anthropics/agentsmesh/proto/gen/go/events/v1"
	runnerv1 "github.com/anthropics/agentsmesh/proto/gen/go/runner/v1"
	"gorm.io/gorm"
)

func setupPodEventCallbacks(db *gorm.DB, podCoordinator *runner.PodCoordinator, eventBus *eventbus.EventBus, notifDispatcher *notifService.Dispatcher) {
	podCoordinator.SetStatusChangeCallback(func(podKey string, status string, agentStatus string) {
		var pod struct {
			OrganizationID int64   `gorm:"column:organization_id"`
			CreatedByID    int64   `gorm:"column:created_by_id"`
			Status         string  `gorm:"column:status"`
			AgentStatus    string  `gorm:"column:agent_status"`
			ErrorCode      *string `gorm:"column:error_code"`
			ErrorMessage   *string `gorm:"column:error_message"`
			TicketSlug     string  `gorm:"column:ticket_slug"`
		}
		if err := db.Table("pods").
			Select("pods.organization_id, pods.created_by_id, pods.status, pods.agent_status, pods.error_code, pods.error_message, COALESCE(tickets.slug, '') AS ticket_slug").
			Joins("LEFT JOIN tickets ON tickets.id = pods.ticket_id").
			Where("pods.pod_key = ?", podKey).
			First(&pod).Error; err != nil {
			slog.Error("failed to get pod for event", "pod_key", podKey, "error", err)
			return
		}

		var eventType eventbus.EventType
		if agentStatus != "" {
			eventType = eventbus.EventPodAgentChanged
		} else if status == agentpod.StatusCompleted || status == agentpod.StatusTerminated {
			eventType = eventbus.EventPodTerminated
		} else if pod.Status == agentpod.StatusInitializing && status == agentpod.StatusRunning {
			eventType = eventbus.EventPodCreated
		} else {
			eventType = eventbus.EventPodStatusChanged
		}

		var event *eventbus.Event
		var err error
		if eventType == eventbus.EventPodCreated {
			created := &eventsv1.PodCreatedEventData{
				PodKey:      podKey,
				Status:      status,
				AgentStatus: agentStatus,
				TicketSlug:  pod.TicketSlug,
			}
			event, err = eventbus.NewEntityEvent(eventType, pod.OrganizationID, "pod", podKey, created)
		} else {
			changed := &eventsv1.PodStatusChangedEventData{
				PodKey:      podKey,
				Status:      status,
				AgentStatus: agentStatus,
			}
			if pod.ErrorCode != nil {
				changed.ErrorCode = *pod.ErrorCode
			}
			if pod.ErrorMessage != nil {
				changed.ErrorMessage = *pod.ErrorMessage
			}
			event, err = eventbus.NewEntityEvent(eventType, pod.OrganizationID, "pod", podKey, changed)
		}
		if err != nil {
			slog.Error("failed to create pod event", "error", err)
			return
		}
		if err := eventBus.Publish(context.Background(), event); err != nil {
			slog.Error("failed to publish pod event", "error", err)
		}

		if status == agentpod.StatusCompleted || status == agentpod.StatusTerminated || status == agentpod.StatusError {
			if wasOSCNotifRecent(podKey) {
				slog.Debug("skipping task:completed notification, recent OSC notification exists", "pod_key", podKey)
			} else if err := notifDispatcher.Dispatch(context.Background(), &notifDomain.NotificationRequest{
				OrganizationID:    pod.OrganizationID,
				Source:            notifDomain.SourceTaskCompleted,
				SourceEntityID:    podKey,
				RecipientResolver: fmt.Sprintf("pod_creator:%s", podKey),
				Title:             "Task Completed",
				Body:              fmt.Sprintf("Pod %s finished with status: %s", podKey, status),
				Link:              fmt.Sprintf("/workspace?pod=%s", podKey),
				Priority:          notifDomain.PriorityNormal,
			}); err != nil {
				slog.Error("failed to dispatch task completed notification", "pod_key", podKey, "error", err)
			}
		}
	})

	podCoordinator.SetInitProgressCallback(func(podKey string, phase string, progress int, message string) {
		var pod struct {
			OrganizationID int64 `gorm:"column:organization_id"`
		}
		if err := db.Table("pods").Where("pod_key = ?", podKey).First(&pod).Error; err != nil {
			slog.Error("failed to get pod for init progress event", "pod_key", podKey, "error", err)
			return
		}

		data := &eventsv1.PodInitProgressEventData{
			PodKey:   podKey,
			Phase:    phase,
			Progress: int32(progress),
			Message:  message,
		}
		event, err := eventbus.NewEntityEvent(eventbus.EventPodInitProgress, pod.OrganizationID, "pod", podKey, data)
		if err != nil {
			slog.Error("failed to create pod init progress event", "error", err)
			return
		}
		if err := eventBus.Publish(context.Background(), event); err != nil {
			slog.Error("failed to publish pod init progress event", "error", err)
		}
	})

	podCoordinator.SetAutopilotStatusChangeCallback(func(
		autopilotKey string,
		podKey string,
		phase string,
		iteration int32,
		maxIterations int32,
		circuitBreakerState string,
		circuitBreakerReason string,
		userTakeover bool,
	) {
		var autopilot struct {
			OrganizationID int64 `gorm:"column:organization_id"`
		}
		if err := db.Table("autopilot_controllers").Where("autopilot_controller_key = ?", autopilotKey).First(&autopilot).Error; err != nil {
			slog.Error("failed to get autopilot for status event", "autopilot_key", autopilotKey, "error", err)
			return
		}

		data := &eventsv1.AutopilotStatusChangedEventData{
			AutopilotControllerKey: autopilotKey,
			PodKey:                 podKey,
			Phase:                  phase,
			CurrentIteration:       iteration,
			MaxIterations:          maxIterations,
			CircuitBreakerState:    circuitBreakerState,
			CircuitBreakerReason:   circuitBreakerReason,
			UserTakeover:           userTakeover,
		}
		event, err := eventbus.NewEntityEvent(eventbus.EventAutopilotStatusChanged, autopilot.OrganizationID, "autopilot_controller", autopilotKey, data)
		if err != nil {
			slog.Error("failed to create autopilot status event", "error", err)
			return
		}
		if err := eventBus.Publish(context.Background(), event); err != nil {
			slog.Error("failed to publish autopilot status event", "error", err)
		}
	})

	podCoordinator.SetAutopilotIterationChangeCallback(func(
		autopilotKey string,
		iteration int32,
		phase string,
		summary string,
		filesChanged []string,
		durationMs int64,
	) {
		var autopilot struct {
			OrganizationID int64 `gorm:"column:organization_id"`
		}
		if err := db.Table("autopilot_controllers").Where("autopilot_controller_key = ?", autopilotKey).First(&autopilot).Error; err != nil {
			slog.Error("failed to get autopilot for iteration event", "autopilot_key", autopilotKey, "error", err)
			return
		}

		data := &eventsv1.AutopilotIterationEventData{
			AutopilotControllerKey: autopilotKey,
			Iteration:              iteration,
			Phase:                  phase,
			Summary:                summary,
			FilesChanged:           filesChanged,
			DurationMs:             durationMs,
		}
		event, err := eventbus.NewEntityEvent(eventbus.EventAutopilotIteration, autopilot.OrganizationID, "autopilot_controller", autopilotKey, data)
		if err != nil {
			slog.Error("failed to create autopilot iteration event", "error", err)
			return
		}
		if err := eventBus.Publish(context.Background(), event); err != nil {
			slog.Error("failed to publish autopilot iteration event", "error", err)
		}
	})

	podCoordinator.SetAutopilotThinkingCallback(func(runnerID int64, protoData *runnerv1.AutopilotThinkingEvent) {
		autopilotKey := protoData.GetAutopilotKey()

		var autopilot struct {
			OrganizationID int64 `gorm:"column:organization_id"`
		}
		if err := db.Table("autopilot_controllers").Where("autopilot_controller_key = ?", autopilotKey).First(&autopilot).Error; err != nil {
			slog.Error("failed to get autopilot for thinking event", "autopilot_key", autopilotKey, "error", err)
			return
		}

		data := protoToEventbusThinking(protoData)

		event, err := eventbus.NewEntityEvent(eventbus.EventAutopilotThinking, autopilot.OrganizationID, "autopilot_controller", autopilotKey, data)
		if err != nil {
			slog.Error("failed to create autopilot thinking event", "error", err)
			return
		}
		if err := eventBus.Publish(context.Background(), event); err != nil {
			slog.Error("failed to publish autopilot thinking event", "error", err)
		}

		slog.Debug("published autopilot thinking event",
			"autopilot_key", autopilotKey,
			"decision_type", data.DecisionType,
			"iteration", data.Iteration)
	})
}

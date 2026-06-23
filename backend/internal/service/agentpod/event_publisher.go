package agentpod

import (
	"context"
	"log/slog"

	"github.com/anthropics/agentsmesh/backend/internal/infra/eventbus"
	eventsv1 "github.com/anthropics/agentsmesh/proto/gen/go/events/v1"
)

type PodEventType string

const (
	PodEventCreated       PodEventType = "created"
	PodEventStatusChanged PodEventType = "status_changed"
	PodEventAgentChanged  PodEventType = "agent_changed"
	PodEventTerminated    PodEventType = "terminated"
)

type EventPublisher interface {
	PublishPodEvent(ctx context.Context, eventType PodEventType, orgID int64, podKey, status, previousStatus, agentStatus string)
}

type EventBusPublisher struct {
	eventBus *eventbus.EventBus
	logger   *slog.Logger
}

func NewEventBusPublisher(eventBus *eventbus.EventBus, logger *slog.Logger) *EventBusPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventBusPublisher{
		eventBus: eventBus,
		logger:   logger.With("component", "pod_event_publisher"),
	}
}

func (p *EventBusPublisher) PublishPodEvent(ctx context.Context, eventType PodEventType, orgID int64, podKey, status, previousStatus, agentStatus string) {
	if p.eventBus == nil {
		return
	}

	var et eventbus.EventType
	switch eventType {
	case PodEventCreated:
		et = eventbus.EventPodCreated
	case PodEventStatusChanged:
		et = eventbus.EventPodStatusChanged
	case PodEventAgentChanged:
		et = eventbus.EventPodAgentChanged
	case PodEventTerminated:
		et = eventbus.EventPodTerminated
	default:
		p.logger.Warn("unknown pod event type", "type", eventType)
		return
	}

	var event *eventbus.Event
	var err error
	if et == eventbus.EventPodCreated {
		event, err = eventbus.NewEntityEvent(et, orgID, "pod", podKey, &eventsv1.PodCreatedEventData{
			PodKey:      podKey,
			Status:      status,
			AgentStatus: agentStatus,
		})
	} else {
		event, err = eventbus.NewEntityEvent(et, orgID, "pod", podKey, &eventsv1.PodStatusChangedEventData{
			PodKey:         podKey,
			Status:         status,
			PreviousStatus: previousStatus,
			AgentStatus:    agentStatus,
		})
	}
	if err != nil {
		p.logger.Error("failed to create pod event",
			"error", err,
			"type", eventType,
			"pod_key", podKey,
		)
		return
	}

	if err := p.eventBus.Publish(ctx, event); err != nil {
		p.logger.Error("failed to publish pod event",
			"error", err,
			"type", eventType,
			"pod_key", podKey,
		)
	}
}

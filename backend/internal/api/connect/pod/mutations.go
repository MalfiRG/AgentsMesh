package podconnect

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect"

	"github.com/anthropics/agentsmesh/backend/internal/api/connect/interceptors"
	podDomain "github.com/anthropics/agentsmesh/backend/internal/domain/agentpod"
	"github.com/anthropics/agentsmesh/backend/internal/domain/grant"
	"github.com/anthropics/agentsmesh/backend/internal/infra/eventbus"
	"github.com/anthropics/agentsmesh/backend/internal/middleware"
	"github.com/anthropics/agentsmesh/backend/internal/service/agentpod"
	"github.com/anthropics/agentsmesh/backend/pkg/policy"
	eventsv1 "github.com/anthropics/agentsmesh/proto/gen/go/events/v1"
	podv1 "github.com/anthropics/agentsmesh/proto/gen/go/pod/v1"
)

// CreatePod — REST analogue: POST /api/v1/organizations/:slug/pods.
// Returns the multi-field {pod, warning?} envelope locked by 986a38ca6.
// The warning surfaces quota-near limits without blocking pod creation.
func (s *Server) CreatePod(
	ctx context.Context, req *connect.Request[podv1.CreatePodRequest],
) (*connect.Response[podv1.CreatePodResponse], error) {
	if s.orchestrator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("pod orchestrator not configured"))
	}
	ctx, _, err := interceptors.ResolveOrgScope(ctx, req.Msg, s.orgSvc)
	if err != nil {
		return nil, err
	}
	tenant := middleware.GetTenant(ctx)

	alias := normalizeAlias(req.Msg.Alias)
	if err := validateAlias(alias); err != nil {
		return nil, err
	}

	orchReq := &agentpod.OrchestrateCreatePodRequest{
		OrganizationID:      tenant.OrganizationID,
		UserID:              tenant.UserID,
		RunnerID:            req.Msg.GetRunnerId(),
		AgentSlug:           req.Msg.GetAgentSlug(),
		RepositoryID:        optionalInt64(req.Msg.RepositoryId),
		TicketSlug:          optionalString(req.Msg.TicketSlug),
		Alias:               alias,
		AgentfileLayer:      optionalString(req.Msg.AgentfileLayer),
		Cols:                req.Msg.GetCols(),
		Rows:                req.Msg.GetRows(),
		SourcePodKey:        req.Msg.GetSourcePodKey(),
		ResumeAgentSession:  optionalBool(req.Msg.ResumeAgentSession),
		Perpetual:           req.Msg.GetPerpetual(),
	}

	result, err := s.orchestrator.CreatePod(ctx, orchReq)
	if err != nil {
		return nil, mapServiceError(err)
	}

	s.publishPodCreated(ctx, result.Pod, req.Msg.GetTicketSlug())

	resp := &podv1.CreatePodResponse{Pod: ToProtoPod(result.Pod)}
	if result.Warning != "" {
		w := result.Warning
		resp.Warning = &w
	}
	return connect.NewResponse(resp), nil
}

// pod:created fired immediately after the orchestrator allocates a Pod
// row. The existing status-callback path (cmd/server/eventbus_pod.go) is
// race-prone: it only fires on initializing → running transitions, which
// a fast-starting mock agent often skips entirely. Publishing here gives
// renderers a deterministic "pod exists" signal independent of runner
// timing. Duplicates with the status-callback path are harmless — handlers
// debounce sidebar refetch.
func (s *Server) publishPodCreated(ctx context.Context, pod *podDomain.Pod, ticketSlug string) {
	if s.eventBus == nil || pod == nil {
		return
	}
	data := &eventsv1.PodCreatedEventData{
		PodKey:      pod.PodKey,
		Status:      pod.Status,
		AgentStatus: pod.AgentStatus,
		RunnerId:    pod.RunnerID,
		CreatedById: pod.CreatedByID,
		TicketSlug:  ticketSlug,
	}
	if pod.TicketID != nil {
		data.TicketId = pod.TicketID
	}
	event, err := eventbus.NewEntityEvent(eventbus.EventPodCreated, pod.OrganizationID, "pod", pod.PodKey, data)
	if err != nil {
		slog.ErrorContext(ctx, "failed to build pod:created event", "pod_key", pod.PodKey, "error", err)
		return
	}
	if err := s.eventBus.Publish(ctx, event); err != nil {
		slog.ErrorContext(ctx, "failed to publish pod:created event", "pod_key", pod.PodKey, "error", err)
	}
}

// TerminatePod — REST analogue: POST /api/v1/organizations/:slug/pods/:key/terminate.
func (s *Server) TerminatePod(
	ctx context.Context, req *connect.Request[podv1.TerminatePodRequest],
) (*connect.Response[podv1.TerminatePodResponse], error) {
	if s.podCoordinator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("pod coordinator not configured"))
	}
	ctx, _, err := interceptors.ResolveOrgScope(ctx, req.Msg, s.orgSvc)
	if err != nil {
		return nil, err
	}
	tenant := middleware.GetTenant(ctx)
	podKey := req.Msg.GetPodKey()

	pod, err := s.podSvc.GetPod(ctx, podKey)
	if err != nil {
		return nil, mapServiceError(err)
	}
	sub := policy.NewSubject(tenant.OrganizationID, tenant.UserID, tenant.UserRole)
	if !policy.PodPolicy.AllowWrite(sub, s.podResourceWithGrants(ctx, podKey, pod.OrganizationID, pod.CreatedByID)) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}

	if err := s.podCoordinator.TerminatePod(ctx, podKey); err != nil {
		return nil, mapServiceError(err)
	}
	if s.grantSvc != nil {
		_ = s.grantSvc.CleanupByResource(ctx, grant.TypePod, podKey)
	}
	return connect.NewResponse(&podv1.TerminatePodResponse{Message: "Pod terminated"}), nil
}

// UpdatePodAlias — REST analogue: PATCH /api/v1/organizations/:slug/pods/:key/alias.
// Alias presence carries clear semantics: absent = no change, empty string = clear.
func (s *Server) UpdatePodAlias(
	ctx context.Context, req *connect.Request[podv1.UpdatePodAliasRequest],
) (*connect.Response[podv1.UpdatePodAliasResponse], error) {
	ctx, _, err := interceptors.ResolveOrgScope(ctx, req.Msg, s.orgSvc)
	if err != nil {
		return nil, err
	}
	tenant := middleware.GetTenant(ctx)
	podKey := req.Msg.GetPodKey()

	pod, err := s.podSvc.GetPod(ctx, podKey)
	if err != nil {
		return nil, mapServiceError(err)
	}
	sub := policy.NewSubject(tenant.OrganizationID, tenant.UserID, tenant.UserRole)
	if !policy.PodPolicy.AllowWrite(sub, s.podResourceWithGrants(ctx, podKey, pod.OrganizationID, pod.CreatedByID)) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}

	alias := optionalString(req.Msg.Alias)
	if alias != nil && strings.TrimSpace(*alias) == "" {
		alias = nil
	}
	if alias != nil && len(*alias) > 100 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("alias must be 100 characters or less"))
	}

	if err := s.podSvc.UpdateAlias(ctx, podKey, alias); err != nil {
		return nil, mapServiceError(err)
	}
	s.publishPodAliasChanged(ctx, pod.OrganizationID, podKey, alias)
	return connect.NewResponse(&podv1.UpdatePodAliasResponse{Message: "Pod alias updated"}), nil
}

// Realtime fan-out for UpdatePodAlias. Renderer / desktop bridge already
// subscribe to pod:alias_changed (see clients/web/src/providers/
// realtimePodHandlers.ts); without this publish the wire stayed empty
// and multi-tab clients fell out of sync until a manual refetch.
func (s *Server) publishPodAliasChanged(ctx context.Context, orgID int64, podKey string, alias *string) {
	if s.eventBus == nil {
		return
	}
	data := &eventsv1.PodAliasChangedEventData{PodKey: podKey}
	if alias != nil {
		data.Alias = alias
	}
	event, err := eventbus.NewEntityEvent(eventbus.EventPodAliasChanged, orgID, "pod", podKey, data)
	if err != nil {
		slog.ErrorContext(ctx, "failed to build pod:alias_changed event", "pod_key", podKey, "error", err)
		return
	}
	if err := s.eventBus.Publish(ctx, event); err != nil {
		slog.ErrorContext(ctx, "failed to publish pod:alias_changed event", "pod_key", podKey, "error", err)
	}
}

// UpdatePodPerpetual — REST analogue: PATCH /api/v1/organizations/:slug/pods/:key/perpetual.
// Members can only edit their own pods; owner/admin can edit any in the org.
func (s *Server) UpdatePodPerpetual(
	ctx context.Context, req *connect.Request[podv1.UpdatePodPerpetualRequest],
) (*connect.Response[podv1.UpdatePodPerpetualResponse], error) {
	ctx, _, err := interceptors.ResolveOrgScope(ctx, req.Msg, s.orgSvc)
	if err != nil {
		return nil, err
	}
	tenant := middleware.GetTenant(ctx)
	podKey := req.Msg.GetPodKey()

	pod, err := s.podSvc.GetPod(ctx, podKey)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if pod.OrganizationID != tenant.OrganizationID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}
	if pod.CreatedByID != tenant.UserID && tenant.UserRole == "member" {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin role required"))
	}

	perpetual := req.Msg.GetPerpetual()
	if err := s.podSvc.UpdatePerpetual(ctx, podKey, perpetual); err != nil {
		return nil, mapServiceError(err)
	}
	if s.commandSender != nil {
		_ = s.commandSender.SendUpdatePodPerpetual(ctx, pod.RunnerID, podKey, perpetual)
	}
	s.publishPodPerpetualChanged(ctx, pod.OrganizationID, podKey, perpetual)
	return connect.NewResponse(&podv1.UpdatePodPerpetualResponse{Message: "Pod perpetual mode updated"}), nil
}

func (s *Server) publishPodPerpetualChanged(ctx context.Context, orgID int64, podKey string, perpetual bool) {
	if s.eventBus == nil {
		return
	}
	data := &eventsv1.PodPerpetualChangedEventData{PodKey: podKey, Perpetual: perpetual}
	event, err := eventbus.NewEntityEvent(eventbus.EventPodPerpetualChanged, orgID, "pod", podKey, data)
	if err != nil {
		slog.ErrorContext(ctx, "failed to build pod:perpetual_changed event", "pod_key", podKey, "error", err)
		return
	}
	if err := s.eventBus.Publish(ctx, event); err != nil {
		slog.ErrorContext(ctx, "failed to publish pod:perpetual_changed event", "pod_key", podKey, "error", err)
	}
}

// SendPodPrompt — REST analogue: POST /api/v1/orgs/:slug/pods/:key/prompt.
// Mode-transparent: PTY writes to stdin, ACP forwards via protocol.
func (s *Server) SendPodPrompt(
	ctx context.Context, req *connect.Request[podv1.SendPodPromptRequest],
) (*connect.Response[podv1.SendPodPromptResponse], error) {
	if s.commandSender == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("command sender not configured"))
	}
	ctx, _, err := interceptors.ResolveOrgScope(ctx, req.Msg, s.orgSvc)
	if err != nil {
		return nil, err
	}
	tenant := middleware.GetTenant(ctx)
	podKey := req.Msg.GetPodKey()

	pod, err := s.podSvc.GetPod(ctx, podKey)
	if err != nil {
		return nil, mapServiceError(err)
	}
	sub := policy.NewSubject(tenant.OrganizationID, tenant.UserID, tenant.UserRole)
	if !policy.PodPolicy.AllowWrite(sub, s.podResourceWithGrants(ctx, podKey, pod.OrganizationID, pod.CreatedByID)) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("forbidden"))
	}
	if !pod.IsActive() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("pod is not active"))
	}

	prompt := req.Msg.GetPrompt()
	if prompt == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("prompt is required"))
	}

	if err := s.commandSender.SendPrompt(ctx, pod.RunnerID, podKey, prompt); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&podv1.SendPodPromptResponse{Status: "sent"}), nil
}

// --- helpers ---

func normalizeAlias(p *string) *string {
	if p == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*p)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateAlias(p *string) error {
	if p != nil && len(*p) > 100 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("alias must be 100 characters or less"))
	}
	return nil
}

func optionalString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func optionalInt64(p *int64) *int64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func optionalBool(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

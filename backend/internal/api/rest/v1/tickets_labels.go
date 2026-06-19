package v1

import (
	"errors"

	ticketDomain "github.com/anthropics/agentsmesh/backend/internal/domain/ticket"
	"github.com/anthropics/agentsmesh/backend/internal/service/ticket"
	"github.com/anthropics/agentsmesh/backend/pkg/apierr"
	"github.com/gin-gonic/gin"
)

// applyLabelUpdate replaces a ticket's labels when the PUT carried a labels
// field (nil = untouched, empty = clear, non-empty = exact set), then re-fetches
// so the response reflects the new set. Returns false after writing an error
// response, in which case the caller must return without emitting a body.
func (h *TicketHandler) applyLabelUpdate(c *gin.Context, t *ticketDomain.Ticket, orgID int64, labels *[]string) (*ticketDomain.Ticket, bool) {
	if labels == nil {
		return t, true
	}
	if err := h.ticketService.ReplaceTicketLabels(c.Request.Context(), t.ID, orgID, *labels); err != nil {
		switch {
		case errors.Is(err, ticketDomain.ErrInvalidLabelName):
			apierr.ValidationError(c, "Invalid label name")
		case errors.Is(err, ticket.ErrTicketNotFound):
			apierr.ResourceNotFound(c, "Ticket not found")
		default:
			apierr.InternalError(c, "Failed to update labels")
		}
		return nil, false
	}
	refreshed, err := h.ticketService.GetTicket(c.Request.Context(), t.ID)
	if err != nil {
		apierr.InternalError(c, "Failed to load updated ticket")
		return nil, false
	}
	return refreshed, true
}

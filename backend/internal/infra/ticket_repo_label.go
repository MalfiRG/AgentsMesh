package infra

import (
	"context"
	"strings"

	"github.com/anthropics/agentsmesh/backend/internal/domain/ticket"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxLabelNameLen = 100

func normalizeLabelNames(names []string) ([]string, error) {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, raw := range names {
		n := strings.TrimSpace(raw)
		if n == "" {
			continue
		}
		if len(n) > maxLabelNameLen {
			return nil, ticket.ErrInvalidLabelName
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

func getOrCreateLabel(tx *gorm.DB, orgID int64, name string) (int64, error) {
	var label ticket.Label
	err := tx.Where("organization_id = ? AND name = ? AND repository_id IS NULL", orgID, name).
		Order("id ASC").First(&label).Error
	if err == nil {
		return label.ID, nil
	}
	if !isNotFound(err) {
		return 0, err
	}
	newLabel := ticket.Label{OrganizationID: orgID, Name: name, Color: "#6B7280"}
	// PG NULLS-distinct: UNIQUE(...,repository_id,...) does not fire for org-level labels; OnConflict is defensive only
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&newLabel).Error; err != nil {
		return 0, err
	}
	if newLabel.ID != 0 {
		return newLabel.ID, nil
	}
	if err := tx.Where("organization_id = ? AND name = ? AND repository_id IS NULL", orgID, name).
		Order("id ASC").First(&label).Error; err != nil {
		return 0, err
	}
	return label.ID, nil
}

func (r *ticketRepository) GetLabelByOrgNameRepo(ctx context.Context, orgID int64, name string, repoID *int64) (*ticket.Label, error) {
	query := r.db.WithContext(ctx).Where("organization_id = ? AND name = ?", orgID, name)
	if repoID != nil {
		query = query.Where("repository_id = ?", *repoID)
	} else {
		query = query.Where("repository_id IS NULL")
	}

	var label ticket.Label
	if err := query.First(&label).Error; err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &label, nil
}

func (r *ticketRepository) CreateLabel(ctx context.Context, label *ticket.Label) error {
	return r.db.WithContext(ctx).Create(label).Error
}

func (r *ticketRepository) GetLabel(ctx context.Context, labelID int64) (*ticket.Label, error) {
	var label ticket.Label
	if err := r.db.WithContext(ctx).First(&label, labelID).Error; err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &label, nil
}

func (r *ticketRepository) ListLabels(ctx context.Context, orgID int64, repoID *int64) ([]*ticket.Label, error) {
	query := r.db.WithContext(ctx).Where("organization_id = ?", orgID)
	if repoID != nil {
		query = query.Where("repository_id IS NULL OR repository_id = ?", *repoID)
	} else {
		query = query.Where("repository_id IS NULL")
	}

	var labels []*ticket.Label
	if err := query.Order("name ASC").Find(&labels).Error; err != nil {
		return nil, err
	}
	return labels, nil
}

func (r *ticketRepository) UpdateLabelFields(ctx context.Context, orgID, labelID int64, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).
		Model(&ticket.Label{}).
		Where("id = ? AND organization_id = ?", labelID, orgID).
		Updates(updates).Error
}

func (r *ticketRepository) DeleteLabelAtomic(ctx context.Context, orgID, labelID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("label_id = ?", labelID).Delete(&ticket.TicketLabel{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND organization_id = ?", labelID, orgID).Delete(&ticket.Label{}).Error
	})
}

func (r *ticketRepository) GetTicketLabels(ctx context.Context, ticketID int64) ([]*ticket.Label, error) {
	var ticketLabels []ticket.TicketLabel
	if err := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID).Find(&ticketLabels).Error; err != nil {
		return nil, err
	}
	if len(ticketLabels) == 0 {
		return []*ticket.Label{}, nil
	}

	ids := make([]int64, len(ticketLabels))
	for i, tl := range ticketLabels {
		ids[i] = tl.LabelID
	}

	var labels []*ticket.Label
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&labels).Error; err != nil {
		return nil, err
	}
	return labels, nil
}

func (r *ticketRepository) AddTicketLabel(ctx context.Context, ticketID, labelID int64) error {
	return r.db.WithContext(ctx).Create(&ticket.TicketLabel{TicketID: ticketID, LabelID: labelID}).Error
}

func (r *ticketRepository) RemoveTicketLabel(ctx context.Context, ticketID, labelID int64) error {
	return r.db.WithContext(ctx).
		Where("ticket_id = ? AND label_id = ?", ticketID, labelID).
		Delete(&ticket.TicketLabel{}).Error
}

func (r *ticketRepository) ReplaceLabels(ctx context.Context, ticketID, orgID int64, names []string) error {
	cleaned, err := normalizeLabelNames(names)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&ticket.Ticket{}).
			Where("id = ? AND organization_id = ?", ticketID, orgID).
			Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Where("ticket_id = ?", ticketID).Delete(&ticket.TicketLabel{}).Error; err != nil {
			return err
		}
		seen := make(map[int64]struct{}, len(cleaned))
		for _, name := range cleaned {
			labelID, err := getOrCreateLabel(tx, orgID, name)
			if err != nil {
				return err
			}
			if _, dup := seen[labelID]; dup {
				continue
			}
			seen[labelID] = struct{}{}
			if err := tx.Create(&ticket.TicketLabel{TicketID: ticketID, LabelID: labelID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

package out

import (
	"context"
	"time"

	"go_pet_project/internal/recurrence/domain"

	"github.com/google/uuid"
)

type DueSeriesHandler func(ctx context.Context, series []domain.TaskSeries) error

type Repository interface {
	Create(ctx context.Context, series domain.TaskSeries) (domain.TaskSeries, error)
	Update(ctx context.Context, series domain.TaskSeries) (domain.TaskSeries, error)
	FindByID(ctx context.Context, id uuid.UUID) (domain.TaskSeries, error)
	ClaimDue(ctx context.Context, now time.Time, limit int, handle DueSeriesHandler) (int, error)
	ListReminderRules(ctx context.Context, seriesID uuid.UUID) ([]domain.ReminderRule, error)
	CreateReminderRule(ctx context.Context, rule domain.ReminderRule) (domain.ReminderRule, error)
}

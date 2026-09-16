package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	recurrenceout "go_pet_project/internal/recurrence/application/port/out"
	recurrencedomain "go_pet_project/internal/recurrence/domain"
	reminderdomain "go_pet_project/internal/reminder/domain"
	taskdomain "go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

type Processor struct {
	series    recurrenceout.Repository
	tasks     recurrenceout.TaskOccurrenceWriter
	reminders recurrenceout.ReminderOccurrenceWriter
	tx        recurrenceout.TransactionRunner
	now       func() time.Time
}

func NewProcessor(
	series recurrenceout.Repository,
	tasks recurrenceout.TaskOccurrenceWriter,
	reminders recurrenceout.ReminderOccurrenceWriter,
	tx recurrenceout.TransactionRunner,
) (*Processor, error) {
	if series == nil {
		return nil, errors.New("recurrence repository is required")
	}
	if tasks == nil {
		return nil, errors.New("task occurrence writer is required")
	}
	if reminders == nil {
		return nil, errors.New("reminder occurrence writer is required")
	}
	if tx == nil {
		return nil, errors.New("transaction runner is required")
	}

	return &Processor{
		series:    series,
		tasks:     tasks,
		reminders: reminders,
		tx:        tx,
		now:       time.Now,
	}, nil
}

func (p *Processor) ProcessDue(ctx context.Context, now time.Time, batchSize int) (int, error) {
	if batchSize <= 0 {
		return 0, errors.New("recurrence batch size must be positive")
	}

	return p.series.ClaimDue(ctx, now, batchSize, func(txCtx context.Context, due []recurrencedomain.TaskSeries) error {
		for _, series := range due {
			if err := p.processOne(txCtx, series); err != nil {
				return err
			}
		}

		return nil
	})
}

func (p *Processor) ProcessSeries(ctx context.Context, series recurrencedomain.TaskSeries) error {
	return p.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		return p.processOne(txCtx, series)
	})
}

func (p *Processor) processOne(ctx context.Context, series recurrencedomain.TaskSeries) error {
	now := p.now().UTC()
	deadline := series.NextDeadlineAt.UTC()
	task, err := taskdomain.NewTaskOccurrence(
		series.ID,
		series.CreatorID,
		series.AssigneeID,
		series.Title,
		series.Description,
		deadline,
		now,
	)
	if err != nil {
		return fmt.Errorf("build occurrence task: %w", err)
	}

	createdTask, err := p.tasks.Create(ctx, task)
	if err != nil {
		return fmt.Errorf("create occurrence task: %w", err)
	}

	rules, err := p.series.ListReminderRules(ctx, series.ID)
	if err != nil {
		return fmt.Errorf("list reminder rules: %w", err)
	}
	for _, rule := range rules {
		if err := p.createReminder(ctx, createdTask.ID, deadline, rule.OffsetSeconds, now); err != nil {
			return err
		}
	}

	if err := series.AdvanceAfterOccurrence(now); err != nil {
		return fmt.Errorf("advance series: %w", err)
	}
	if _, err := p.series.Update(ctx, series); err != nil {
		return fmt.Errorf("update series: %w", err)
	}

	return nil
}

func (p *Processor) createReminder(ctx context.Context, taskID uuid.UUID, deadline time.Time, offsetSeconds int64, now time.Time) error {
	reminder, err := reminderdomain.NewBeforeDeadlineReminder(uuid.New(), taskID, deadline, offsetSeconds, now)
	if err != nil {
		return fmt.Errorf("build occurrence reminder: %w", err)
	}
	if _, err := p.reminders.Create(ctx, reminder); err != nil {
		return fmt.Errorf("create occurrence reminder: %w", err)
	}

	return nil
}

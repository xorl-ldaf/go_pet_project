package postgres

import (
	"fmt"
	"time"

	"go_pet_project/internal/recurrence/domain"
)

func seriesToModel(series domain.TaskSeries) seriesModel {
	return seriesModel{
		ID:             series.ID,
		CreatorID:      series.CreatorID,
		AssigneeID:     series.AssigneeID,
		Title:          series.Title,
		Description:    series.Description,
		Frequency:      string(series.Frequency),
		Interval:       series.Interval,
		NextDeadlineAt: series.NextDeadlineAt,
		Timezone:       series.Timezone,
		EndsAt:         cloneTimePtr(series.EndsAt),
		IsActive:       series.IsActive,
		CreatedAt:      series.CreatedAt,
		UpdatedAt:      series.UpdatedAt,
	}
}

func seriesToDomain(model seriesModel) (domain.TaskSeries, error) {
	frequency, err := domain.ParseFrequency(model.Frequency)
	if err != nil {
		return domain.TaskSeries{}, fmt.Errorf("map series model to domain: %w", err)
	}

	series, err := domain.RestoreTaskSeries(
		model.ID,
		model.CreatorID,
		model.AssigneeID,
		model.Title,
		model.Description,
		frequency,
		model.Interval,
		model.NextDeadlineAt,
		model.Timezone,
		cloneTimePtr(model.EndsAt),
		model.IsActive,
		model.CreatedAt,
		model.UpdatedAt,
	)
	if err != nil {
		return domain.TaskSeries{}, fmt.Errorf("map series model to domain: %w", err)
	}

	return series, nil
}

func reminderRuleToModel(rule domain.ReminderRule) reminderRuleModel {
	return reminderRuleModel{
		ID:            rule.ID,
		SeriesID:      rule.SeriesID,
		OffsetSeconds: rule.OffsetSeconds,
	}
}

func reminderRuleToDomain(model reminderRuleModel) (domain.ReminderRule, error) {
	rule, err := domain.RestoreReminderRule(model.ID, model.SeriesID, model.OffsetSeconds)
	if err != nil {
		return domain.ReminderRule{}, fmt.Errorf("map reminder rule model to domain: %w", err)
	}

	return rule, nil
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}

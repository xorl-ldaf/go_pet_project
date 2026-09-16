package httpadapter

import "time"

type CreateReminderRequest struct {
	Kind          string     `json:"kind"`
	OffsetSeconds *int64     `json:"offset_seconds"`
	TriggerAt     *time.Time `json:"trigger_at"`
}

type UpdateReminderRequest struct {
	OffsetSeconds *int64     `json:"offset_seconds"`
	TriggerAt     *time.Time `json:"trigger_at"`
}

func (r UpdateReminderRequest) IsEmpty() bool {
	return r.OffsetSeconds == nil && r.TriggerAt == nil
}

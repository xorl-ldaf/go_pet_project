package httpadapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type CreateTaskRequest struct {
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DeadlineAt  *time.Time `json:"deadline_at"`
}

type UpdateTaskRequest struct {
	AssigneeID  *uuid.UUID    `json:"assignee_id"`
	Title       *string       `json:"title"`
	Description *string       `json:"description"`
	DeadlineAt  DeadlinePatch `json:"deadline_at"`
}

func (r UpdateTaskRequest) IsEmpty() bool {
	return r.AssigneeID == nil && r.Title == nil && r.Description == nil && !r.DeadlineAt.Set
}

type DeadlinePatch struct {
	Set   bool
	Value *time.Time
}

func (p *DeadlinePatch) UnmarshalJSON(data []byte) error {
	p.Set = true
	if string(data) == "null" {
		p.Value = nil
		return nil
	}

	var value time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("deadline_at must be RFC3339 timestamp or null: %w", err)
	}
	p.Value = &value

	return nil
}

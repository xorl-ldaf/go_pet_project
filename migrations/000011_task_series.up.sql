CREATE TABLE task_series (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_id UUID NOT NULL,
    assignee_id UUID NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    frequency TEXT NOT NULL,
    "interval" INTEGER NOT NULL,
    next_deadline_at TIMESTAMPTZ NOT NULL,
    timezone TEXT NOT NULL,
    ends_at TIMESTAMPTZ NULL,
    is_active BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT task_series_creator_id_fkey
        FOREIGN KEY (creator_id)
        REFERENCES users (id),

    CONSTRAINT task_series_assignee_id_fkey
        FOREIGN KEY (assignee_id)
        REFERENCES users (id),

    CONSTRAINT task_series_frequency_check
        CHECK (frequency IN ('DAILY', 'WEEKLY', 'MONTHLY')),

    CONSTRAINT task_series_interval_positive_check
        CHECK ("interval" > 0)
);

CREATE INDEX task_series_due_idx
    ON task_series (next_deadline_at)
    WHERE is_active = true;

CREATE TABLE task_series_reminder_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    series_id UUID NOT NULL,
    offset_seconds BIGINT NOT NULL,

    CONSTRAINT task_series_reminder_rules_series_id_fkey
        FOREIGN KEY (series_id)
        REFERENCES task_series (id),

    CONSTRAINT task_series_reminder_rules_offset_positive_check
        CHECK (offset_seconds > 0)
);

CREATE INDEX task_series_reminder_rules_series_id_idx
    ON task_series_reminder_rules (series_id);

ALTER TABLE tasks
    ADD CONSTRAINT tasks_series_id_fkey
    FOREIGN KEY (series_id)
    REFERENCES task_series (id);

CREATE UNIQUE INDEX tasks_series_deadline_unique_idx
    ON tasks (series_id, deadline_at)
    WHERE series_id IS NOT NULL;

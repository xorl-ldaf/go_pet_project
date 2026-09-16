CREATE TABLE reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL,
    kind TEXT NOT NULL,
    offset_seconds BIGINT NULL,
    trigger_at TIMESTAMPTZ NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at TIMESTAMPTZ NULL,

    CONSTRAINT reminders_task_id_fkey
        FOREIGN KEY (task_id)
        REFERENCES tasks (id)
);

CREATE INDEX reminders_task_id_idx ON reminders (task_id);
CREATE INDEX reminders_state_trigger_at_idx ON reminders (state, trigger_at);

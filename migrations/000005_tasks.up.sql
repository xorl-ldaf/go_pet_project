CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    series_id UUID NULL,
    creator_id UUID NOT NULL,
    assignee_id UUID NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    deadline_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ NULL,
    archived_at TIMESTAMPTZ NULL,

    CONSTRAINT tasks_creator_id_fkey
        FOREIGN KEY (creator_id)
        REFERENCES users (id),

    CONSTRAINT tasks_assignee_id_fkey
        FOREIGN KEY (assignee_id)
        REFERENCES users (id)
);

CREATE INDEX tasks_assignee_status_deadline_idx ON tasks (assignee_id, status, deadline_at);
CREATE INDEX tasks_creator_created_at_idx ON tasks (creator_id, created_at);
CREATE INDEX tasks_deadline_at_idx ON tasks (deadline_at);

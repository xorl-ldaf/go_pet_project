CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    task_id UUID NULL,
    reminder_id UUID NULL,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    read_at TIMESTAMPTZ NULL,

    CONSTRAINT notifications_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users (id),
    CONSTRAINT notifications_task_id_fkey
        FOREIGN KEY (task_id)
        REFERENCES tasks (id),
    CONSTRAINT notifications_reminder_id_fkey
        FOREIGN KEY (reminder_id)
        REFERENCES reminders (id)
);

CREATE INDEX notifications_user_created_at_idx
    ON notifications (user_id, created_at);

CREATE INDEX notifications_user_unread_created_at_idx
    ON notifications (user_id, created_at)
    WHERE read_at IS NULL;

CREATE TABLE processed_events (
    consumer_name TEXT NOT NULL,
    event_id UUID NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT processed_events_pkey
        PRIMARY KEY (consumer_name, event_id)
);

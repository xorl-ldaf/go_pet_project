CREATE TABLE telegram_links (
    user_id UUID PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    telegram_username TEXT NULL,
    linked_at TIMESTAMPTZ NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,

    CONSTRAINT telegram_links_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE CASCADE
);

CREATE TABLE telegram_link_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT telegram_link_tokens_user_id_fkey
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE CASCADE
);

CREATE INDEX telegram_link_tokens_expires_at_idx
    ON telegram_link_tokens (expires_at);

CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL,
    channel TEXT NOT NULL,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NULL,
    sent_at TIMESTAMPTZ NULL,
    last_error TEXT NULL,

    CONSTRAINT notification_deliveries_notification_id_fkey
        FOREIGN KEY (notification_id)
        REFERENCES notifications (id)
        ON DELETE CASCADE,
    CONSTRAINT notification_deliveries_notification_channel_key
        UNIQUE (notification_id, channel),
    CONSTRAINT notification_deliveries_attempts_non_negative
        CHECK (attempts >= 0)
);

CREATE INDEX notification_deliveries_due_idx
    ON notification_deliveries (next_attempt_at)
    WHERE status = 'PENDING';

CREATE TABLE assignment_permissions (
    assigner_id UUID NOT NULL,
    assignee_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT assignment_permissions_pkey
        PRIMARY KEY (assigner_id, assignee_id),

    CONSTRAINT assignment_permissions_assigner_id_fkey
        FOREIGN KEY (assigner_id)
        REFERENCES users (id),

    CONSTRAINT assignment_permissions_assignee_id_fkey
        FOREIGN KEY (assignee_id)
        REFERENCES users (id)
);

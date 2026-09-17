DROP INDEX IF EXISTS tasks_series_deadline_unique_idx;

ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS tasks_series_id_fkey;

DROP INDEX IF EXISTS task_series_reminder_rules_series_id_idx;
DROP TABLE IF EXISTS task_series_reminder_rules;

DROP INDEX IF EXISTS task_series_due_idx;
DROP TABLE IF EXISTS task_series;

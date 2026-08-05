-- +goose Up
-- Per-sandbox security policy. mode + denied_commands are agent-launch concerns
-- carried to the worker via WorkerLaunchSpec; auto_stop_minutes is a lifetime cap
-- applied by the reconciler when provisioning the sandbox. Defaults preserve the
-- current behavior (trusted/unrestricted, no denied commands, 30-minute auto-stop)
-- so existing sessions are unchanged.
ALTER TABLE ao_sessions
    ADD COLUMN mode TEXT NOT NULL DEFAULT 'trusted';

ALTER TABLE ao_sessions
    ADD COLUMN denied_commands TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE ao_sandboxes
    ADD COLUMN auto_stop_minutes INTEGER NOT NULL DEFAULT 30;

-- +goose Down
ALTER TABLE ao_sandboxes DROP COLUMN auto_stop_minutes;
ALTER TABLE ao_sessions DROP COLUMN denied_commands;
ALTER TABLE ao_sessions DROP COLUMN mode;

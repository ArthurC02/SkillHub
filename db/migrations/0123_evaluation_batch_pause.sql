ALTER TABLE evaluation_batches
    DROP CONSTRAINT evaluation_batches_status_check,
    DROP CONSTRAINT evaluation_batches_stop_has_reason,
    ADD COLUMN paused_at timestamptz,
    ADD CONSTRAINT evaluation_batches_status_check CHECK (status IN ('launching', 'launched', 'paused', 'stopped')),
    ADD CONSTRAINT evaluation_batches_stop_has_reason CHECK ((status IN ('paused', 'stopped')) = (stop_reason IS NOT NULL)),
    ADD CONSTRAINT evaluation_batches_paused_has_time CHECK ((status = 'paused') = (paused_at IS NOT NULL));

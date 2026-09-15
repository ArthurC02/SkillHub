ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_event_type_check;

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_event_type_check CHECK (event_type IN (
        'run.queued',
        'run.provisioning',
        'run.preparing',
        'run.running',
        'run.evaluating',
        'run.succeeded',
        'run.failed',
        'run.cancelled',
        'run.timed_out',
        'run.cleanup_cleaned',
        'run.cleanup_failed',
        'evaluation.started',
        'evaluation.superseded',
        'evaluation.completed',
        'evaluation.failed',
        'evaluation.feedback_recorded',
        'evaluation.suggestion_decided',
        'skill.taken_down',
        'skill.access_restricted',
        'skill.access_restriction_lifted',
        'skill.redistribution_set',
        'skill.categorized',
        'skill.deleted',
        'skill.created',
        'skill.version_added',
        'skill.described'
    ));

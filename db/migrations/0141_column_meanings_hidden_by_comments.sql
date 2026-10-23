COMMENT ON COLUMN trace_events.occurred_at IS
    'When the producer''s own clock says the event happened. Clocks differ between producers, so within one '
    '(run, attempt, producer) stream seq is the order and occurred_at only merges streams; an event is refused '
    'when this lies too far in the future or before the retention window.';

COMMENT ON COLUMN analytics_events.result_count IS
    'For a search event, how many results the search returned; NULL for events that are not searches. '
    'has_results is true exactly when this is greater than zero.';

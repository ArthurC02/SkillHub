ALTER TABLE cost_events DROP CONSTRAINT cost_events_kind_check;
ALTER TABLE cost_events ADD CONSTRAINT cost_events_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'match_reasons'));

ALTER TABLE cost_statistics DROP CONSTRAINT cost_statistics_kind_check;
ALTER TABLE cost_statistics ADD CONSTRAINT cost_statistics_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run', 'creation_session', 'match_reasons'));

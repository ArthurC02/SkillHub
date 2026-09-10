-- A Run's gateway spend is a cost kind; both tables share the kind vocabulary.

ALTER TABLE cost_events DROP CONSTRAINT cost_events_kind_check;
ALTER TABLE cost_events ADD CONSTRAINT cost_events_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run'));

ALTER TABLE cost_statistics DROP CONSTRAINT cost_statistics_kind_check;
ALTER TABLE cost_statistics ADD CONSTRAINT cost_statistics_kind_check CHECK (kind IN (
    'creation_step', 'search_embedding', 'index_enrich',
    'review', 'suggestion', 'generate', 'run'));

ALTER TABLE skill_sources ADD COLUMN content_changed_at timestamptz;

COMMENT ON COLUMN skill_sources.content_changed_at IS
    'First sweep on which a re-fetch hashed differently from content_hash. NULL means every check so far matched, or no check has compared content yet. Never cleared: the snapshot we hold does not become current again.';

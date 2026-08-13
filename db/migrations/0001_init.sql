-- 0001_init: enable the extensions every later migration assumes (ADR-014).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
CREATE EXTENSION IF NOT EXISTS vector;

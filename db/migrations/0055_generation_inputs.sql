ALTER TABLE skill_sources ADD COLUMN generation_inputs jsonb;

COMMENT ON COLUMN skill_sources.generation_inputs IS
    'Inputs beyond task_description that produced a generated package (ADR-066): the diagram''s digest, media type and byte count, and the reference skills'' ids and names. NULL for git, upload and text-only generations. Never the image bytes.';

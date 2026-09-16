ALTER TABLE skill_sources ADD COLUMN counts_toward_generate_quota boolean;

UPDATE skill_sources
SET counts_toward_generate_quota = source_type = 'generated'
    AND NOT COALESCE(generation_inputs @> '{"interactive": true}', false);

UPDATE skill_sources
SET generation_inputs = generation_inputs - 'interactive'
WHERE generation_inputs ? 'interactive';

ALTER TABLE skill_sources ALTER COLUMN counts_toward_generate_quota SET NOT NULL;

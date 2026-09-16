ALTER TABLE search_documents ADD COLUMN has_script boolean;

UPDATE search_documents
SET has_script = scan->'codes' @> '["script-file"]'::jsonb OR scan->'codes' @> '["embedded-script"]'::jsonb
WHERE scan ? 'codes';

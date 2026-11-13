CREATE FUNCTION pg_temp.creation_messages_without_fetch_text(messages jsonb)
RETURNS jsonb LANGUAGE SQL AS $$
    SELECT CASE WHEN jsonb_typeof(messages) = 'array' THEN (
        SELECT coalesce(jsonb_agg(
            CASE WHEN message->>'role' = 'tool'
                      AND jsonb_typeof(observation->'fetch') = 'object'
                      AND observation->'fetch' ? 'text'
                 THEN jsonb_set(message, '{content}', to_jsonb((observation #- '{fetch,text}')::text))
                 ELSE message
            END ORDER BY ordinal
        ), '[]'::jsonb)
        FROM jsonb_array_elements(messages) WITH ORDINALITY AS entries(message, ordinal)
        CROSS JOIN LATERAL (
            SELECT CASE WHEN message->>'content' IS JSON OBJECT
                        THEN regexp_replace(message->>'content', '(?<!\\)((?:\\\\)*)\\u0000', '\1\\ufffd', 'g')::jsonb
                   END AS observation
        ) parsed
    ) ELSE messages END
$$;

DO $$
DECLARE
    previous_purge_setting text := current_setting('skillhub.purge', true);
BEGIN
    PERFORM set_config('skillhub.purge', 'on', true);

UPDATE creation_sessions
SET snapshot = jsonb_set(snapshot, '{snapshot,messages}', pg_temp.creation_messages_without_fetch_text(snapshot #> '{snapshot,messages}'))
WHERE jsonb_typeof(snapshot #> '{snapshot,messages}') = 'array'
  AND snapshot #> '{snapshot,messages}' IS DISTINCT FROM pg_temp.creation_messages_without_fetch_text(snapshot #> '{snapshot,messages}');

UPDATE creation_receipts
SET result = jsonb_set(result, '{snapshot,messages}', pg_temp.creation_messages_without_fetch_text(result #> '{snapshot,messages}'))
WHERE jsonb_typeof(result #> '{snapshot,messages}') = 'array'
  AND result #> '{snapshot,messages}' IS DISTINCT FROM pg_temp.creation_messages_without_fetch_text(result #> '{snapshot,messages}');

    PERFORM set_config('skillhub.purge', coalesce(previous_purge_setting, ''), true);
END;
$$;

DROP FUNCTION pg_temp.creation_messages_without_fetch_text(jsonb);

-- Runnable check for the CORE-004 immutability triggers (0005_immutability.sql).
-- Usage: psql -v ON_ERROR_STOP=1 -f db/tests/immutability_test.sql
-- Everything runs inside one transaction and is rolled back at the end.
BEGIN;

CREATE FUNCTION must_fail(stmt text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    BEGIN
        EXECUTE stmt;
    EXCEPTION WHEN restrict_violation THEN
        RETURN;
    END;
    RAISE EXCEPTION 'expected immutability rejection but statement succeeded: %', stmt;
END;
$$;

-- Fixtures.
INSERT INTO users (id, email, display_name)
VALUES ('11111111-1111-1111-1111-111111111111', 'a@example.test', 'A');
INSERT INTO workspaces (id, owner_user_id, name)
VALUES ('22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 'personal');
INSERT INTO skills (id, workspace_id, name)
VALUES ('33333333-3333-3333-3333-333333333333', '22222222-2222-2222-2222-222222222222', 'demo');
INSERT INTO skill_versions (id, workspace_id, skill_id, version_number, content_hash, package_object_key)
VALUES ('44444444-4444-4444-4444-444444444444', '22222222-2222-2222-2222-222222222222',
        '33333333-3333-3333-3333-333333333333', 1, 'hash-1', 'ws/22/skill/33/v1.tar.zst');
INSERT INTO test_cases (id, workspace_id, skill_id, name, user_prompt)
VALUES ('55555555-5555-5555-5555-555555555555', '22222222-2222-2222-2222-222222222222',
        '33333333-3333-3333-3333-333333333333', 'tc', 'summarise this');
INSERT INTO test_case_snapshots (id, workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
VALUES ('66666666-6666-6666-6666-666666666666', '22222222-2222-2222-2222-222222222222',
        '55555555-5555-5555-5555-555555555555', 'summarise this', '[]'::jsonb, 'hash-tc-1');
INSERT INTO runs (id, workspace_id, skill_version_id, test_case_snapshot_id, provider)
VALUES ('77777777-7777-7777-7777-777777777777', '22222222-2222-2222-2222-222222222222',
        '44444444-4444-4444-4444-444444444444', '66666666-6666-6666-6666-666666666666', 'self-hosted');

-- 1. skill_versions are frozen (iron rule 4).
SELECT must_fail($$UPDATE skill_versions SET content_hash = 'tampered' WHERE content_hash = 'hash-1'$$);
SELECT must_fail($$DELETE FROM skill_versions WHERE content_hash = 'hash-1'$$);

-- 2. test case snapshots are frozen.
SELECT must_fail($$UPDATE test_case_snapshots SET user_prompt = 'edited' WHERE content_hash = 'hash-tc-1'$$);
SELECT must_fail($$DELETE FROM test_case_snapshots WHERE content_hash = 'hash-tc-1'$$);

-- 3. A non-terminal run is still writable, and its transitions are logged.
UPDATE runs SET status = 'running', started_at = now()
WHERE id = '77777777-7777-7777-7777-777777777777';
INSERT INTO run_status_transitions (run_id, workspace_id, from_status, to_status, reason)
VALUES ('77777777-7777-7777-7777-777777777777', '22222222-2222-2222-2222-222222222222',
        'queued', 'running', 'provisioned');

-- 4. Transition log is append only.
SELECT must_fail($$UPDATE run_status_transitions SET reason = 'rewritten' WHERE to_status = 'running'$$);
SELECT must_fail($$DELETE FROM run_status_transitions WHERE to_status = 'running'$$);

-- 5. Trace events are append only, and land in the monthly partition.
INSERT INTO trace_events (workspace_id, run_id, seq, occurred_at, event_type, source)
VALUES ('22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
        1, '2026-08-14 10:00:00+00', 'skill_activated', 'agent');
DO $$
BEGIN
    IF (SELECT count(*) FROM trace_events_2026_08) <> 1 THEN
        RAISE EXCEPTION 'trace event did not route into trace_events_2026_08';
    END IF;
END;
$$;
SELECT must_fail($$UPDATE trace_events SET event_type = 'rewritten' WHERE seq = 1$$);
SELECT must_fail($$DELETE FROM trace_events WHERE seq = 1$$);

-- 6. A terminal run freezes, except for the cleanup columns (ADR-004).
UPDATE runs SET status = 'succeeded', finished_at = now()
WHERE id = '77777777-7777-7777-7777-777777777777';
SELECT must_fail($$UPDATE runs SET status = 'failed' WHERE id = '77777777-7777-7777-7777-777777777777'$$);
SELECT must_fail($$UPDATE runs SET runtime_snapshot = '{"model":"swapped"}'::jsonb WHERE id = '77777777-7777-7777-7777-777777777777'$$);
SELECT must_fail($$DELETE FROM runs WHERE id = '77777777-7777-7777-7777-777777777777'$$);
UPDATE runs SET cleanup_status = 'cleaned', cleanup_at = now()
WHERE id = '77777777-7777-7777-7777-777777777777';

-- 7. Duplicate content does not create a second version (SKILL-001).
DO $$
BEGIN
    BEGIN
        INSERT INTO skill_versions (workspace_id, skill_id, version_number, content_hash, package_object_key)
        VALUES ('22222222-2222-2222-2222-222222222222', '33333333-3333-3333-3333-333333333333',
                2, 'hash-1', 'ws/22/skill/33/v2.tar.zst');
    EXCEPTION WHEN unique_violation THEN
        RETURN;
    END;
    RAISE EXCEPTION 'expected duplicate content_hash to be rejected';
END;
$$;

\echo 'immutability_test: OK'
ROLLBACK;

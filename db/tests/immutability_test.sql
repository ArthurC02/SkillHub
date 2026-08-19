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

-- Separate from must_fail on purpose: a CHECK raises check_violation, and folding
-- it into must_fail would let a constraint failure pass as proof of an
-- immutability trigger that never fired.
CREATE FUNCTION must_violate_check(stmt text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    BEGIN
        EXECUTE stmt;
    EXCEPTION WHEN check_violation THEN
        RETURN;
    END;
    RAISE EXCEPTION 'expected a check constraint rejection but statement succeeded: %', stmt;
END;
$$;

-- Third helper for the same reason must_violate_check is separate from must_fail:
-- a composite foreign key raises foreign_key_violation, and letting that count as
-- either of the other two would let one guard stand in as proof of another.
CREATE FUNCTION must_violate_fk(stmt text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    BEGIN
        EXECUTE stmt;
    EXCEPTION WHEN foreign_key_violation THEN
        RETURN;
    END;
    RAISE EXCEPTION 'expected a foreign key rejection but statement succeeded: %', stmt;
END;
$$;

CREATE FUNCTION must_violate_unique(stmt text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    BEGIN
        EXECUTE stmt;
    EXCEPTION WHEN unique_violation THEN
        RETURN;
    END;
    RAISE EXCEPTION 'expected a uniqueness rejection but statement succeeded: %', stmt;
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

-- 3. A non-terminal run is still writable, legal transitions are logged, and
-- direct SQL cannot bypass the lifecycle matrix.
SELECT must_violate_check($$UPDATE runs SET status = 'running'
WHERE id = '77777777-7777-7777-7777-777777777777'$$);
UPDATE runs SET status = 'provisioning', started_at = now()
WHERE id = '77777777-7777-7777-7777-777777777777';
INSERT INTO run_status_transitions (run_id, workspace_id, from_status, to_status, reason)
VALUES ('77777777-7777-7777-7777-777777777777', '22222222-2222-2222-2222-222222222222',
        'queued', 'provisioning', 'provisioned');

-- 4. Transition log is append only.
SELECT must_fail($$UPDATE run_status_transitions SET reason = 'rewritten' WHERE to_status = 'provisioning'$$);
SELECT must_fail($$DELETE FROM run_status_transitions WHERE to_status = 'provisioning'$$);

-- 5. Trace events are append only, and land in the monthly partition.
INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
                          event_type, source, masked)
VALUES ('88888888-8888-4888-8888-888888888888',
        '22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
        1, 1, '2026-08-14 10:00:00+00', 'skill_activation', 'sandbox', true);
DO $$
BEGIN
    IF (SELECT count(*) FROM trace_events_2026_08) <> 1 THEN
        RAISE EXCEPTION 'trace event did not route into trace_events_2026_08';
    END IF;
END;
$$;
SELECT must_fail($$UPDATE trace_events SET event_type = 'rewritten' WHERE seq = 1$$);
SELECT must_fail($$DELETE FROM trace_events WHERE seq = 1$$);

-- 5a. Iron rule 11 at the storage layer (0019): an unmasked event cannot be written
-- at all, so no code path can skip the masker (TRACE-005).
SELECT must_violate_check($$
    INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
                              event_type, source, masked)
    VALUES ('99999999-9999-4999-8999-999999999999',
            '22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
            1, 2, '2026-08-14 10:00:01+00', 'agent_output', 'sandbox', false)
$$);

-- 5b. Redelivery is a no-op, not a second row (TRACE-008): the producer's event_id
-- is the idempotency key and at-least-once delivery makes repeats routine.
INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
                          event_type, source, masked)
VALUES ('88888888-8888-4888-8888-888888888888',
        '22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
        1, 1, '2026-08-14 10:00:00+00', 'skill_activation', 'sandbox', true)
ON CONFLICT (event_id, occurred_at) DO NOTHING;
DO $$
BEGIN
    IF (SELECT count(*) FROM trace_events) <> 1 THEN
        RAISE EXCEPTION 'a redelivered trace event was stored twice';
    END IF;
END;
$$;

-- A different event cannot reuse a logical stream ordinal, even in another
-- time partition. event_id idempotency alone does not protect gap detection.
SELECT must_violate_unique($$
    INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
                              event_type, source, masked)
    VALUES ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
            '22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
            1, 1, '2027-03-14 10:00:00+00', 'agent_output', 'sandbox', true)
$$);

-- 5c. An event outside the pre-created month still lands, in the default partition.
-- Without it the first run in a month nobody created a partition for would lose its
-- whole trace (0019 partitioning note).
INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
                          event_type, source, masked)
VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
        '22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
        1, 2, '2027-03-14 10:00:00+00', 'agent_output', 'sandbox', true);
DO $$
BEGIN
    IF (SELECT count(*) FROM trace_events_default) <> 1 THEN
        RAISE EXCEPTION 'an out-of-range trace event did not reach the default partition';
    END IF;
END;
$$;

-- 6. A terminal run freezes, except for the cleanup columns (ADR-004).
UPDATE runs SET status = 'preparing'
WHERE id = '77777777-7777-7777-7777-777777777777';
UPDATE runs SET status = 'running'
WHERE id = '77777777-7777-7777-7777-777777777777';
UPDATE runs SET status = 'evaluating'
WHERE id = '77777777-7777-7777-7777-777777777777';
UPDATE runs SET status = 'succeeded', finished_at = now()
WHERE id = '77777777-7777-7777-7777-777777777777';
SELECT must_fail($$UPDATE runs SET status = 'failed' WHERE id = '77777777-7777-7777-7777-777777777777'$$);
SELECT must_fail($$UPDATE runs SET runtime_snapshot = '{"model":"swapped"}'::jsonb WHERE id = '77777777-7777-7777-7777-777777777777'$$);
SELECT must_fail($$DELETE FROM runs WHERE id = '77777777-7777-7777-7777-777777777777'$$);
UPDATE runs SET cleanup_status = 'cleaned', cleanup_at = now()
WHERE id = '77777777-7777-7777-7777-777777777777';

-- 6a. The retention purge escape hatch (0013_governance): DELETE opens only for
-- a transaction that set the flag, UPDATE never does, and the flag is gone again
-- as soon as it is reset. An audit event is used as the subject because it is the
-- newest append-only table and covered by the same shared trigger.
INSERT INTO audit_events (action, resource_type) VALUES ('test.event', 'test');
SELECT must_fail($$UPDATE audit_events SET action = 'tampered' WHERE action = 'test.event'$$);
SELECT must_fail($$DELETE FROM audit_events WHERE action = 'test.event'$$);
SET LOCAL skillhub.purge = 'on';
SELECT must_fail($$UPDATE audit_events SET action = 'tampered' WHERE action = 'test.event'$$);
DELETE FROM audit_events WHERE action = 'test.event';
SET LOCAL skillhub.purge = 'off';
INSERT INTO audit_events (action, resource_type) VALUES ('test.event', 'test');
SELECT must_fail($$DELETE FROM audit_events WHERE action = 'test.event'$$);

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

-- 8. Evaluations (0024): re-evaluation is append-only, one current verdict per
-- run, and a completed verdict freezes except for the user's feedback.
INSERT INTO evaluations (id, workspace_id, run_id, status, overall, evidence_complete)
VALUES ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', '22222222-2222-2222-2222-222222222222',
        '77777777-7777-7777-7777-777777777777', 'pending', 'undetermined', false);

-- Still writable while the evaluation job is running.
UPDATE evaluations SET status = 'completed', overall = 'met', evidence_complete = true,
       evaluated_at = now(), judge_model = 'gpt-5.6-terra', judge_prompt_version = 'judge-1'
WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';

SELECT must_fail($$UPDATE evaluations SET overall = 'not_met' WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'$$);
SELECT must_fail($$DELETE FROM evaluations WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'$$);
-- The user may change their mind about a verdict without changing the verdict.
UPDATE evaluations SET feedback_helpful = true, feedback_comment = 'useful', updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';

-- 8a. A second *current* evaluation for the same run is refused.
DO $$
BEGIN
    BEGIN
        INSERT INTO evaluations (workspace_id, run_id, status, overall, evidence_complete)
        VALUES ('22222222-2222-2222-2222-222222222222',
                '77777777-7777-7777-7777-777777777777', 'completed', 'not_met', true);
    EXCEPTION WHEN unique_violation THEN
        RETURN;
    END;
    RAISE EXCEPTION 'expected a second current evaluation to be rejected';
END;
$$;

-- 8b. Superseding the previous verdict makes room for the re-evaluation, and
-- any number of superseded ones coexist - the history is what is being kept.
UPDATE evaluations SET superseded_at = now() WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
INSERT INTO evaluations (id, workspace_id, run_id, status, overall, evidence_complete, superseded_at)
VALUES ('cccccccc-cccc-4ccc-8ccc-cccccccccccc', '22222222-2222-2222-2222-222222222222',
        '77777777-7777-7777-7777-777777777777', 'completed', 'partially_met', true, now());
INSERT INTO evaluations (id, workspace_id, run_id, status, overall, evidence_complete)
VALUES ('dddddddd-dddd-4ddd-8ddd-dddddddddddd', '22222222-2222-2222-2222-222222222222',
        '77777777-7777-7777-7777-777777777777', 'completed', 'met', true);
DO $$
BEGIN
    IF (SELECT count(*) FROM evaluations WHERE run_id = '77777777-7777-7777-7777-777777777777') <> 3 THEN
        RAISE EXCEPTION 're-evaluation did not keep the superseded verdicts';
    END IF;
    IF (SELECT count(*) FROM evaluations
        WHERE run_id = '77777777-7777-7777-7777-777777777777' AND superseded_at IS NULL) <> 1 THEN
        RAISE EXCEPTION 'expected exactly one current evaluation';
    END IF;
END;
$$;

-- 8c. An evidence ref without its excerpt/availability would be unresolvable once
-- the trace partition is dropped, and these rows can never be repaired (0024).
SELECT must_violate_check($$
    INSERT INTO evaluations (workspace_id, run_id, status, overall, evidence_complete, criterion_results)
    VALUES ('22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
            'completed', 'met', true,
            '[{"criterion_id":"c1","result":"passed","source":"model",
               "evidence":[{"kind":"trace_event","trace_event_id":"88888888-8888-4888-8888-888888888888"}]}]'::jsonb)
$$);

-- 8d. A suggestion's content is frozen; only the human decision on it moves.
INSERT INTO evaluation_suggestions (id, workspace_id, evaluation_id, category, problem,
                                    target_path, proposed_content, expected_impact)
VALUES ('eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee', '22222222-2222-2222-2222-222222222222',
        'dddddddd-dddd-4ddd-8ddd-dddddddddddd', 'skill', 'SKILL.md never mentions CSV',
        'SKILL.md', 'add a CSV section', 'the skill activates on CSV prompts');
SELECT must_fail($$UPDATE evaluation_suggestions SET problem = 'rewritten' WHERE id = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'$$);
SELECT must_fail($$DELETE FROM evaluation_suggestions WHERE id = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'$$);
UPDATE evaluation_suggestions
SET decision = 'accepted', decided_at = now(),
    applied_skill_version_id = '44444444-4444-4444-4444-444444444444'
WHERE id = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee';

-- 8e. A path that escapes the package cannot even be stored (design §5.2 floor).
SELECT must_violate_check($$
    INSERT INTO evaluation_suggestions (workspace_id, evaluation_id, category, problem,
                                        target_path, proposed_content, expected_impact)
    VALUES ('22222222-2222-2222-2222-222222222222', 'dddddddd-dddd-4ddd-8ddd-dddddddddddd',
            'skill', 'p', '../../etc/passwd', 'c', 'i')
$$);

-- 9. Packaging (0027): redistribution is refused by default, a download package
-- is a fact, and its download history is append only.

-- 9a. Every existing skill starts blocked, because "nobody classified this yet"
-- must not read as permission to redistribute it (DISC-003, ADR-021 §5.3).
DO $$
BEGIN
    IF (SELECT redistribution FROM skills WHERE id = '33333333-3333-3333-3333-333333333333')
       <> 'unknown' THEN
        RAISE EXCEPTION 'a skill did not default to redistribution = unknown';
    END IF;
END;
$$;
SELECT must_violate_check($$
    UPDATE skills SET redistribution = 'source-available'
    WHERE id = '33333333-3333-3333-3333-333333333333'
$$);

-- 9b. Packaging attributes only attach to a download package, and only inside the
-- artifact's own workspace - both are copied columns, checked by composite FK.
INSERT INTO artifacts (id, workspace_id, run_id, kind, file_name, content_type,
                       size_bytes, content_hash, object_key, expires_at)
VALUES ('f1111111-1111-4111-8111-111111111111', '22222222-2222-2222-2222-222222222222',
        NULL, 'download_package', 'demo-standard.zip', 'application/zip',
        1024, 'sha256-pkg-1', 'ws/22/downloads/f1.zip', now() + interval '90 days');
INSERT INTO artifacts (id, workspace_id, run_id, kind, file_name, content_type,
                       size_bytes, content_hash, object_key, expires_at)
VALUES ('f2222222-2222-4222-8222-222222222222', '22222222-2222-2222-2222-222222222222',
        '77777777-7777-7777-7777-777777777777', 'run_output', 'out.txt', 'text/plain',
        12, 'sha256-out-1', 'ws/22/runs/77/out.txt', now() + interval '30 days');

SELECT must_violate_fk($$
    INSERT INTO download_artifacts (artifact_id, workspace_id, skill_version_id, target,
                                    profile_version, packager_version, manifest_hash,
                                    includes_test_cases)
    VALUES ('f2222222-2222-4222-8222-222222222222', '22222222-2222-2222-2222-222222222222',
            '44444444-4444-4444-4444-444444444444', 'standard', '1', 'pkg-1', 'sha256-m-1', false)
$$);

INSERT INTO download_artifacts (artifact_id, workspace_id, skill_version_id, target,
                                profile_version, packager_version, manifest_hash,
                                includes_test_cases)
VALUES ('f1111111-1111-4111-8111-111111111111', '22222222-2222-2222-2222-222222222222',
        '44444444-4444-4444-4444-444444444444', 'standard', '1', 'pkg-1', 'sha256-m-1', false);

-- 9c. Repackaging is a new row, never an edit (iron rule 4). The mutable half of a
-- download lives on artifacts, which is not frozen.
SELECT must_fail($$UPDATE download_artifacts SET manifest_hash = 'sha256-swapped'
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);
SELECT must_fail($$DELETE FROM download_artifacts
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);
UPDATE artifacts SET scan_status = 'available'
WHERE id = 'f1111111-1111-4111-8111-111111111111';

-- 9d. The download history is append only (WS-002 1): "you downloaded this on that
-- date" is not editable state.
INSERT INTO download_records (workspace_id, artifact_id, actor_user_id)
VALUES ('22222222-2222-2222-2222-222222222222', 'f1111111-1111-4111-8111-111111111111',
        '11111111-1111-1111-1111-111111111111');
SELECT must_fail($$UPDATE download_records SET downloaded_at = now() - interval '1 day'
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);
SELECT must_fail($$DELETE FROM download_records
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);

\echo 'immutability_test: OK'
ROLLBACK;

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

-- Passes only on check_violation, so a CHECK failure cannot pass as proof of a trigger.
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

-- Passes only on foreign_key_violation, so it cannot stand in for the other guards.
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

-- Passes only when the message matches, because a trigger that raises without an
-- ERRCODE shares raise_exception with every unrelated failure in this file.
CREATE FUNCTION must_fail_saying(stmt text, fragment text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    BEGIN
        EXECUTE stmt;
    EXCEPTION WHEN raise_exception THEN
        IF position(fragment IN SQLERRM) = 0 THEN
            RAISE EXCEPTION 'rejected for the wrong reason (%): %', SQLERRM, stmt;
        END IF;
        RETURN;
    END;
    RAISE EXCEPTION 'expected immutability rejection but statement succeeded: %', stmt;
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

SELECT must_fail($$UPDATE skill_versions SET content_hash = 'tampered' WHERE content_hash = 'hash-1'$$);
SELECT must_fail($$DELETE FROM skill_versions WHERE content_hash = 'hash-1'$$);

SELECT must_fail($$UPDATE test_case_snapshots SET user_prompt = 'edited' WHERE content_hash = 'hash-tc-1'$$);
SELECT must_fail($$DELETE FROM test_case_snapshots WHERE content_hash = 'hash-tc-1'$$);

SELECT must_violate_check($$UPDATE runs SET status = 'running'
WHERE id = '77777777-7777-7777-7777-777777777777'$$);
UPDATE runs SET status = 'provisioning', started_at = now()
WHERE id = '77777777-7777-7777-7777-777777777777';
INSERT INTO run_status_transitions (run_id, workspace_id, from_status, to_status, reason)
VALUES ('77777777-7777-7777-7777-777777777777', '22222222-2222-2222-2222-222222222222',
        'queued', 'provisioning', 'provisioned');

SELECT must_fail($$UPDATE run_status_transitions SET reason = 'rewritten' WHERE to_status = 'provisioning'$$);
SELECT must_fail($$DELETE FROM run_status_transitions WHERE to_status = 'provisioning'$$);

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

SELECT must_violate_check($$
    INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
                              event_type, source, masked)
    VALUES ('99999999-9999-4999-8999-999999999999',
            '22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
            1, 2, '2026-08-14 10:00:01+00', 'agent_output', 'sandbox', false)
$$);

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

SELECT must_violate_unique($$
    INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at,
                              event_type, source, masked)
    VALUES ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
            '22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
            1, 1, '2027-03-14 10:00:00+00', 'agent_output', 'sandbox', true)
$$);

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

INSERT INTO audit_events (action, resource_type) VALUES ('test.event', 'test');
SELECT must_fail($$UPDATE audit_events SET action = 'tampered' WHERE action = 'test.event'$$);
SELECT must_fail($$DELETE FROM audit_events WHERE action = 'test.event'$$);
SET LOCAL skillhub.purge = 'on';
SELECT must_fail($$UPDATE audit_events SET action = 'tampered' WHERE action = 'test.event'$$);
DELETE FROM audit_events WHERE action = 'test.event';
SET LOCAL skillhub.purge = 'off';
INSERT INTO audit_events (action, resource_type) VALUES ('test.event', 'test');
SELECT must_fail($$DELETE FROM audit_events WHERE action = 'test.event'$$);

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

INSERT INTO evaluations (id, workspace_id, run_id, status, overall, evidence_complete)
VALUES ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', '22222222-2222-2222-2222-222222222222',
        '77777777-7777-7777-7777-777777777777', 'pending', 'undetermined', false);

UPDATE evaluations SET status = 'completed', overall = 'met', evidence_complete = true,
       evaluated_at = now(), judge_model = 'gpt-5.6-terra', judge_prompt_version = 'judge-1'
WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';

SELECT must_fail($$UPDATE evaluations SET overall = 'not_met' WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'$$);
SELECT must_fail($$DELETE FROM evaluations WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'$$);
UPDATE evaluations SET feedback_helpful = true, feedback_comment = 'useful', updated_at = now()
WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';

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

SELECT must_violate_check($$
    INSERT INTO evaluations (workspace_id, run_id, status, overall, evidence_complete, criterion_results)
    VALUES ('22222222-2222-2222-2222-222222222222', '77777777-7777-7777-7777-777777777777',
            'completed', 'met', true,
            '[{"criterion_id":"c1","result":"passed","source":"model",
               "evidence":[{"kind":"trace_event","trace_event_id":"88888888-8888-4888-8888-888888888888"}]}]'::jsonb)
$$);

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

SELECT must_violate_check($$
    INSERT INTO evaluation_suggestions (workspace_id, evaluation_id, category, problem,
                                        target_path, proposed_content, expected_impact)
    VALUES ('22222222-2222-2222-2222-222222222222', 'dddddddd-dddd-4ddd-8ddd-dddddddddddd',
            'skill', 'p', '../../etc/passwd', 'c', 'i')
$$);

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

SELECT must_fail($$UPDATE download_artifacts SET manifest_hash = 'sha256-swapped'
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);
SELECT must_fail($$DELETE FROM download_artifacts
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);
UPDATE artifacts SET scan_status = 'available'
WHERE id = 'f1111111-1111-4111-8111-111111111111';

INSERT INTO download_records (workspace_id, artifact_id, actor_user_id)
VALUES ('22222222-2222-2222-2222-222222222222', 'f1111111-1111-4111-8111-111111111111',
        '11111111-1111-1111-1111-111111111111');
SELECT must_fail($$UPDATE download_records SET downloaded_at = now() - interval '1 day'
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);
SELECT must_fail($$DELETE FROM download_records
                   WHERE artifact_id = 'f1111111-1111-4111-8111-111111111111'$$);

SET LOCAL skillhub.purge = 'on';
DELETE FROM download_records WHERE workspace_id = '22222222-2222-2222-2222-222222222222';
DELETE FROM download_artifacts WHERE workspace_id = '22222222-2222-2222-2222-222222222222';
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM download_artifacts
               WHERE workspace_id = '22222222-2222-2222-2222-222222222222')
       OR EXISTS (SELECT 1 FROM download_records
                  WHERE workspace_id = '22222222-2222-2222-2222-222222222222') THEN
        RAISE EXCEPTION 'the purge flag did not open the download tables for deletion';
    END IF;
END;
$$;
SET LOCAL skillhub.purge = 'off';

INSERT INTO run_attempts (id, run_id, workspace_id, attempt_number, provider)
VALUES ('a0000000-0000-4000-8000-000000000001', '77777777-7777-7777-7777-777777777777',
        '22222222-2222-2222-2222-222222222222', 1, 'self-hosted');

UPDATE run_attempts SET finished_at = now(), object_grants_state = 'closed'
WHERE id = 'a0000000-0000-4000-8000-000000000001';
DO $$
BEGIN
    IF (SELECT finished_at IS NULL OR object_grants_state <> 'closed' FROM run_attempts
        WHERE id = 'a0000000-0000-4000-8000-000000000001') THEN
        RAISE EXCEPTION 'the attempt guard refused a column it lists as mutable';
    END IF;
END;
$$;
SELECT must_fail($$UPDATE run_attempts SET attempt_number = 2
                   WHERE id = 'a0000000-0000-4000-8000-000000000001'$$);
SELECT must_fail($$UPDATE run_attempts SET provider = 'swapped'
                   WHERE id = 'a0000000-0000-4000-8000-000000000001'$$);
SELECT must_fail($$DELETE FROM run_attempts
                   WHERE id = 'a0000000-0000-4000-8000-000000000001'$$);

INSERT INTO evaluation_model_usage (evaluation_id, workspace_id, operation, model,
                                    prompt_version, prompt_tokens, completion_tokens,
                                    cost_usd, cost_source)
VALUES ('dddddddd-dddd-4ddd-8ddd-dddddddddddd', '22222222-2222-2222-2222-222222222222',
        'judge', 'gpt-5.6-terra', 'judge-1', 1200, 300, 0.004500, 'gateway');
SELECT must_fail($$UPDATE evaluation_model_usage SET prompt_tokens = 1
                   WHERE operation = 'judge'$$);
SELECT must_fail($$DELETE FROM evaluation_model_usage WHERE operation = 'judge'$$);

INSERT INTO evaluation_suggestion_applications (workspace_id, suggestion_id, skill_version_id)
VALUES ('22222222-2222-2222-2222-222222222222', 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee',
        '44444444-4444-4444-4444-444444444444');
SELECT must_fail($$UPDATE evaluation_suggestion_applications
                   SET skill_version_id = '44444444-4444-4444-4444-444444444444',
                       applied_at = now() - interval '1 day'
                   WHERE suggestion_id = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'$$);
SELECT must_fail($$DELETE FROM evaluation_suggestion_applications
                   WHERE suggestion_id = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee'$$);

INSERT INTO credit_accounts (user_id, balance_credits)
VALUES ('11111111-1111-1111-1111-111111111111', 100);
INSERT INTO cost_events (id, kind, model, prompt_version, prompt_tokens, completion_tokens,
                         usd_micros, cost_source, workspace_id, user_id, ref_type, ref_id,
                         idempotency_key)
VALUES ('c0000000-0000-4000-8000-000000000001', 'run', 'gpt-5.6-terra', 'run-1', 900, 120,
        3400, 'gateway', '22222222-2222-2222-2222-222222222222',
        '11111111-1111-1111-1111-111111111111', 'run',
        '77777777-7777-7777-7777-777777777777', 'cost-run-77-1');
INSERT INTO credit_entries (id, user_id, kind, delta_credits, usd_micros, markup_bps, model,
                            prompt_version, ref_type, ref_id, cost_event_id, idempotency_key)
VALUES ('c0000000-0000-4000-8000-000000000002', '11111111-1111-1111-1111-111111111111',
        'debit', -4, 3400, 2000, 'gpt-5.6-terra', 'run-1', 'run',
        '77777777-7777-7777-7777-777777777777', 'c0000000-0000-4000-8000-000000000001',
        'entry-run-77-1');

SELECT must_fail($$UPDATE cost_events SET usd_micros = 1
                   WHERE idempotency_key = 'cost-run-77-1'$$);
SELECT must_fail($$DELETE FROM cost_events WHERE idempotency_key = 'cost-run-77-1'$$);
SELECT must_fail($$UPDATE credit_entries SET delta_credits = -1
                   WHERE idempotency_key = 'entry-run-77-1'$$);
SELECT must_fail($$UPDATE credit_entries SET markup_bps = 500
                   WHERE idempotency_key = 'entry-run-77-1'$$);
SELECT must_fail($$DELETE FROM credit_entries WHERE idempotency_key = 'entry-run-77-1'$$);

SET LOCAL skillhub.purge = 'on';
DELETE FROM credit_entries WHERE idempotency_key = 'entry-run-77-1';
DELETE FROM cost_events WHERE idempotency_key = 'cost-run-77-1';
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM credit_entries WHERE idempotency_key = 'entry-run-77-1')
       OR EXISTS (SELECT 1 FROM cost_events WHERE idempotency_key = 'cost-run-77-1') THEN
        RAISE EXCEPTION 'the purge flag did not open the ledger tables for deletion';
    END IF;
END;
$$;
SET LOCAL skillhub.purge = 'off';

INSERT INTO creation_sessions (id, workspace_id, state, revision, snapshot, expires_at)
VALUES ('e0000000-0000-4000-8000-000000000001', '22222222-2222-2222-2222-222222222222',
        'gathering', 1, '{}'::jsonb, now() + interval '7 days');
INSERT INTO creation_session_events (session_id, workspace_id, revision, event_type, snapshot)
VALUES ('e0000000-0000-4000-8000-000000000001', '22222222-2222-2222-2222-222222222222',
        1, 'session_started', '{"state":"gathering"}'::jsonb);
SELECT must_fail_saying($$UPDATE creation_session_events SET snapshot = '{"state":"rewritten"}'::jsonb
                          WHERE session_id = 'e0000000-0000-4000-8000-000000000001'$$,
                        'immutable');
SELECT must_fail_saying($$UPDATE creation_session_events SET event_type = 'rewritten'
                          WHERE session_id = 'e0000000-0000-4000-8000-000000000001'$$,
                        'immutable');

INSERT INTO publishers (id, workspace_id, name)
VALUES ('f0000000-0000-4000-8000-000000000001', '22222222-2222-2222-2222-222222222222', 'demo-publisher');
INSERT INTO publications (id, publisher_id, name, skill_id, status)
VALUES ('f0000000-0000-4000-8000-000000000002', 'f0000000-0000-4000-8000-000000000001',
        'demo', '33333333-3333-3333-3333-333333333333', 'published');
INSERT INTO publication_releases (id, publication_id, skill_version_id, version_number, content_hash,
                                  findings, rights_attested, released_by)
VALUES ('f0000000-0000-4000-8000-000000000003', 'f0000000-0000-4000-8000-000000000002',
        '44444444-4444-4444-4444-444444444444', 1, 'hash-1', '{}'::jsonb, true,
        '11111111-1111-1111-1111-111111111111');
SELECT must_fail($$UPDATE publication_releases SET content_hash = 'hash-rewritten'
                   WHERE id = 'f0000000-0000-4000-8000-000000000003'$$);
SELECT must_fail($$UPDATE publication_releases SET rights_attested = false
                   WHERE id = 'f0000000-0000-4000-8000-000000000003'$$);
SELECT must_fail($$DELETE FROM publication_releases WHERE id = 'f0000000-0000-4000-8000-000000000003'$$);
UPDATE publications SET status = 'delisted' WHERE id = 'f0000000-0000-4000-8000-000000000002';

\echo 'immutability_test: OK'
ROLLBACK;

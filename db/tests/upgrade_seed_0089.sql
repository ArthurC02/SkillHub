INSERT INTO users (id, email, display_name)
VALUES ('11111111-1111-1111-1111-111111111111', 'upgrade@example.test', 'Upgrade');
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
VALUES ('77777777-7777-7777-7777-777777777771', '22222222-2222-2222-2222-222222222222',
        '44444444-4444-4444-4444-444444444444', '66666666-6666-6666-6666-666666666666', 'self-hosted'),
       ('77777777-7777-7777-7777-777777777772', '22222222-2222-2222-2222-222222222222',
        '44444444-4444-4444-4444-444444444444', '66666666-6666-6666-6666-666666666666', 'self-hosted');
UPDATE runs SET status = 'failed', finished_at = created_at + interval '1 minute'
WHERE id = '77777777-7777-7777-7777-777777777772';

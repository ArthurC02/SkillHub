-- 0030_dispatch_halts: the one switch that stops dispatching new Runs
-- (02:SEC-010 P1 的自動第一動作 / 03:SEC-012, and ADR-022 X-04 的 drain 與暫停派送).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- 03:SEC-012 makes sharing this state an implementation requirement rather than a
-- note: 「ADR-022 X-04 的『單節點 ≥50% slot drain／全池 ≥25%（下限 2 筆）暫停整池派送』
-- 與 P1 的『停止派送』是同一個開關的兩個觸發者，必須共用同一份狀態與同一條解除路徑」.
-- So there is exactly one table, both triggers INSERT into it, both releases are the
-- same UPDATE, and "is the platform dispatching right now" has one answer.
--
-- Why a table and not an environment variable or a file:
--   * The API process refuses new runs and the worker process refuses to dispatch.
--     A variable would have to be set in both and they restart independently, which
--     is precisely the split-brain 03:SEC-012 forbids.
--   * The X-04 trigger is the reconciler, a River leader-only periodic job whose
--     leadership moves between worker processes. It has to be able to flip the
--     switch without a deploy, and a restart must not un-flip it.
--   * A P1 halt has no automatic release (03:SEC-012 「解除不得是自動的」), so the
--     state has to outlive every process that can observe it.
--
-- No workspace_id, and that is not an iron-rule-3 exception: a halt is a property of
-- the execution plane, not of anybody's data. It takes no user input and returns no
-- user content.
CREATE TABLE dispatch_halts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Empty string is the whole pool; a provider name is one node drained
    -- (ADR-022 X-04 ①「該節點停止接受新 Run（drain）」, ②「暫停整個節點池派送」).
    -- One column and not scope+provider: the two are never independent, and a
    -- 'pool' scope with a provider name filled in is a state nothing should have
    -- to interpret.
    provider    text NOT NULL DEFAULT '',
    -- Which trigger declared it. It is data about one halt, NOT a second switch:
    -- every reader treats an active row the same way for dispatch. It decides
    -- exactly three things, each of which the two acceptance criteria differ on:
    --   * 'p1_incident'      — preserves the scene (cleanup and orphan teardown
    --                          stand down, 02:SEC-010 「保留現場」), refuses run
    --                          creation outright, and is NEVER lifted automatically.
    --   * 'orphan_threshold' — ADR-022 X-04. Cleanup must keep running (it is what
    --                          clears the orphans that raised it), new runs stay
    --                          `queued` as X-04 says, and it self-clears after two
    --                          consecutive rounds below the threshold.
    source      text NOT NULL CHECK (source IN ('p1_incident', 'orphan_threshold')),
    -- Why, in the declarer's words, or the computed threshold breach. Required and
    -- not satisfiable with whitespace, same rule as 02:SEC-011 「理由（必填，空字串
    -- 不成立）」 — a halt of the whole platform that nobody can explain later is not
    -- a decision.
    reason      text NOT NULL CHECK (btrim(reason) <> ''),
    -- NULL for a halt the platform declared itself (the X-04 reconciler). The same
    -- convention audit.Event uses for a platform-initiated event.
    declared_by uuid REFERENCES users (id) ON DELETE SET NULL,
    declared_at timestamptz NOT NULL DEFAULT now(),
    -- Consecutive reconciler rounds this halt's own condition has been clear.
    -- ADR-022 X-04 「恢復條件：遺留筆數降到門檻以下且連續 2 輪維持，才自動恢復派送
    -- ——不做單輪恢復，避免在清理不穩定時來回抖動」. Lives on the row rather than in
    -- the reconciler's memory for the same reason 0021's sightings table does: the
    -- job is leader-elected and leadership moves.
    clear_rounds integer NOT NULL DEFAULT 0 CHECK (clear_rounds >= 0),
    -- Lifted, by whom, and why. The row is kept: a halt is an incident record, and
    -- 「現在到底有沒有在派送」 is answered by lifted_at IS NULL, not by a row's
    -- existence.
    lifted_at   timestamptz,
    lifted_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    lift_reason text,
    CHECK ((lifted_at IS NULL) = (lift_reason IS NULL))
);

-- One live halt per target. This is what makes the two triggers share the switch
-- instead of stacking two of them: a second declaration on the same target is an
-- upsert onto this row, never a new one, so lifting it really does resume dispatch.
CREATE UNIQUE INDEX dispatch_halts_active_target_idx
    ON dispatch_halts (provider) WHERE lifted_at IS NULL;

COMMENT ON TABLE dispatch_halts IS
    'Active rows stop new Runs being dispatched (03:SEC-012 P1 first action, ADR-022 X-04 drain/suspend). provider = '''' is the whole pool. Shared by both triggers on purpose: one state, one release path. See 0030.';

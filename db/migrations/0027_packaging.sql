-- 0027_packaging: what a download package is, who downloaded it, and whether the
-- skill was ever allowed to be handed out at all (PACK-001, WS-004, SEC-007).
-- Shape and reasoning: docs/plans/mvp/m4/contract-deltas.md §4,
-- docs/plans/mvp/m4/packaging-design.md §2.4, §4.5, §7.2.
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- What this migration deliberately does NOT touch: `artifacts` keeps its shape
-- (0004 already reserved kind = 'download_package', a nullable run_id "NULL for
-- packaging downloads", the three scan states and expires_at), so there is no
-- second artifacts table here — only the columns that generic table cannot hold.
-- `runs`, `evaluations` and the 0005/0024 triggers are untouched: packaging
-- neither creates nor modifies a skill version (iron rule 4).
--
-- `artifacts.expires_at` stays as it is on purpose. PDM-006's 90 days for a
-- download package is proposed, not ratified (m4/README §8.1), so the value is
-- computed by the writer from deployment config; a DEFAULT here would turn an
-- unratified proposal into schema.

-- (a) can this skill be handed out at all -------------------------------------
-- Until now the database could not answer "may a Download Artifact be produced
-- from this skill". The policy existed in three places, none of them here:
-- tools/content/seed-skills.json's curation field, a comment in catalog/trust.go,
-- and the line in 0023 saying "Download packaging is unaffected here" — true only
-- for as long as there was no packaging.
--
-- Two locks, opposite directions, both required (packaging-design §4.5):
-- `access_restriction` (0023) is a hold somebody pressed by hand and covers the
-- four rows known today; this column is a property of the content, and every
-- skill needs an answer to it including the one a user uploads tomorrow. Treating
-- the hold as the redistribution test would mean "nobody objected, so it may be
-- copied" — the direction ADR-021 §5.3 records a real misjudgement in.
--
-- On `skills` and not `skill_versions`, for the reason 0023 already settled: a
-- licence covers the source, so it covers every version, and this verdict has to
-- be revisable — skill_versions is immutable (iron rule 4).
--
-- Default 'unknown' and not 'allowed': unknown blocks. DISC-003 forbids implying
-- an unknown licence may be redistributed, and the existing rows are exactly the
-- population nobody has classified yet. A CHECK and not an enum, because unlike
-- 0023's open-ended reason codes this really is a closed three-valued judgement.
ALTER TABLE skills
    ADD COLUMN redistribution text NOT NULL DEFAULT 'unknown'
        CHECK (redistribution IN ('allowed', 'blocked', 'unknown'));

COMMENT ON COLUMN skills.redistribution IS
    'May a Download Artifact be produced from this skill? Only ''allowed'' releases; ''unknown'' blocks. license_status = Confirmed must never set this on its own (CONTENT-002). Copied onto forks at fork time, like access_restriction. See 0027.';

-- (b) the packaging half of an artifacts row ----------------------------------
-- Referenced by the composite foreign key below. Not just id: it pins the two
-- facts a packaging side row must not contradict — that the artifact it hangs off
-- is a download package and not a run output, and that it names the same
-- workspace. Both are copied columns, and a copied column that nothing checks
-- drifts (iron rule 3).
CREATE UNIQUE INDEX artifacts_packaging_ref_key ON artifacts (id, kind, workspace_id);

CREATE TABLE download_artifacts (
    artifact_id uuid PRIMARY KEY,
    -- Constant, and it exists only to be matched by the FK below. A run output
    -- can therefore never acquire packaging attributes.
    kind text NOT NULL DEFAULT 'download_package' CHECK (kind = 'download_package'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),

    -- The immutable version these bytes were built from. Packaging reads it and
    -- writes nothing back (iron rule 4). skill_id is deliberately not copied: it
    -- is one join away and would be a third thing able to drift.
    skill_version_id uuid NOT NULL REFERENCES skill_versions (id),

    -- The packaging target ('standard' | a profile id) and the version of its
    -- configuration. No CHECK on the value set: PDM-008 is not ratified
    -- (m4/README §8.1) and the authoritative list is the versioned profile
    -- configuration under contracts/packaging/, so a CHECK here would freeze an
    -- unratified list into the schema and make each new profile a migration.
    target          text NOT NULL CHECK (btrim(target) <> ''),
    profile_version text NOT NULL CHECK (btrim(profile_version) <> ''),
    -- Which build produced it. content_hash (on artifacts) is only reproducible
    -- within one packager version — that is why this is recorded rather than
    -- inferred (packaging-design §2.4).
    packager_version text NOT NULL CHECK (btrim(packager_version) <> ''),

    -- The canonical hash of {path: sha256(bytes)}, excluding the manifest itself
    -- and all zip metadata. Separate from artifacts.content_hash on purpose: that
    -- one answers "is this the same file", this one answers "is this the same
    -- content", and zip mtimes make those two different questions
    -- (ADR-012 可重現性, packaging-design §2.4).
    manifest_hash text NOT NULL CHECK (btrim(manifest_hash) <> ''),

    -- PACK-005. Part of the identity of a package, not a display flag: it is one
    -- of the four fields that decide whether a repeat request is the same request.
    includes_test_cases boolean NOT NULL,

    CONSTRAINT download_artifacts_artifact_fkey
        FOREIGN KEY (artifact_id, kind, workspace_id)
        REFERENCES artifacts (id, kind, workspace_id)
);

-- Serves both the workspace-scoped listing and the download_records FK below.
CREATE UNIQUE INDEX download_artifacts_workspace_key
    ON download_artifacts (workspace_id, artifact_id);
-- The idempotency lookup of POST /packaging (contract-deltas §1.1): same version,
-- same target, same packager, same test-case choice. Not unique — "already has
-- one" also requires the artifact to be available and unexpired, and both of
-- those live on artifacts and change with time, which no index predicate can
-- express. So the key is a lookup the writer resolves, not a constraint.
CREATE INDEX download_artifacts_dedupe_idx
    ON download_artifacts (skill_version_id, target, packager_version, includes_test_cases);

-- A produced package is a fact: the same four fields that identify it also
-- determine every other column, so nothing on this row has a legitimate second
-- value. Repackaging is a new row (iron rule 4); the mutable half of a download
-- (scan_status, expires_at, deleted_at) lives on artifacts and is untouched here.
CREATE TRIGGER download_artifacts_immutable
    BEFORE UPDATE OR DELETE ON download_artifacts
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

-- (c) who downloaded what, and when -------------------------------------------
-- WS-002 1's "download history", which is a product feature the user reads. Kept
-- apart from audit_events (0013) although both record a download: the audit trail
-- is a compliance record with a 400 day retention that stores identifiers only,
-- this one is shown to the owner and outlives the artifact's bytes. Merging them
-- would bind each to the other's stricter rule (packaging-design §7.2).
CREATE TABLE download_records (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    artifact_id  uuid NOT NULL,
    -- NOT NULL: there is no anonymous download. The endpoint takes its scope from
    -- the session (iron rule 3), so every row has an actor. The FK survives
    -- account purge, which de-identifies the users row in place (0013).
    actor_user_id uuid NOT NULL REFERENCES users (id),
    downloaded_at timestamptz NOT NULL DEFAULT now(),

    -- Which version and which profile were downloaded (WS-004) are read through
    -- this reference rather than copied. Composite so a record cannot name an
    -- artifact belonging to another workspace.
    CONSTRAINT download_records_artifact_fkey
        FOREIGN KEY (artifact_id, workspace_id)
        REFERENCES download_artifacts (artifact_id, workspace_id)
);

-- The workspace's download history, newest first.
CREATE INDEX download_records_workspace_id_idx
    ON download_records (workspace_id, downloaded_at DESC);
-- download_count and "when was this last fetched" for one artifact.
CREATE INDEX download_records_artifact_id_idx
    ON download_records (artifact_id, downloaded_at DESC);

-- Append only, same shared trigger as every other log of things that happened
-- (0005, 0013). "You downloaded this on that date" is not editable state, and a
-- history that can be rewritten is not a history. The retention purge reaches it
-- through the same skillhub.purge flag as the rest.
CREATE TRIGGER download_records_immutable
    BEFORE UPDATE OR DELETE ON download_records
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

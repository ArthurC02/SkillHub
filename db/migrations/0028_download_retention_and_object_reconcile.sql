-- 0028_download_retention_and_object_reconcile: the two ways stored bytes stop
-- existing, and the sweep that notices when the database has not been told
-- (SEC-006, CORE-007, 04 丙-9).
-- Applied migrations are immutable: fix forward with a new file, never edit this one.
--
-- Nothing here is new user-facing state. It is the bookkeeping that lets two
-- claims the platform makes stay true: "this artifact is downloadable" and
-- "this run's inputs could be supplied again".

-- (a) bytes gone, row kept ------------------------------------------------------
-- `artifacts` already has `deleted_at`, and it means one specific thing: the
-- OWNER deleted this, so 02:SEC-006's 「完成後不再出現在一般存取介面」 applies and
-- the row must disappear from ordinary listings.
--
-- Retention expiry is a different fact with a different consequence. The bytes go
-- but the row stays visible, because public.yaml's GET /downloads says in as many
-- words that an expired package must not vanish from the history silently — "it
-- expired" and "it never existed" are different answers to 02:WS-002 1. Writing
-- deleted_at from the retention sweep would fold those two facts into one column
-- and the listing could no longer tell them apart.
--
-- It is also what makes the sweep finite. Without a mark, every pass would have
-- to re-issue a delete for every artifact that ever expired, forever.
--
-- The object-existence reconciler writes it too, for the third way bytes stop
-- existing: they were lost. Same consequence — the row must stop claiming they
-- are there — so it is the same column and not a second one.
ALTER TABLE artifacts ADD COLUMN purged_at timestamptz;

COMMENT ON COLUMN artifacts.purged_at IS
    'When the stored object was removed while the row was kept: retention expiry, or a reconciler finding the bytes gone. Distinct from deleted_at, which is the owner deleting it and hides the row (SEC-006). Serving the bytes requires this to be NULL. See 0028.';

-- Serves the retention sweep's worklist. Partial, because the set it scans is
-- "expired and not yet purged", which shrinks to nothing between passes — an
-- unqualified index on expires_at would grow with every artifact ever made.
CREATE INDEX artifacts_unpurged_expiry_idx ON artifacts (expires_at)
    WHERE purged_at IS NULL AND deleted_at IS NULL;

-- (b) the object-existence reconciler's state ----------------------------------
-- 04 丙-9. `RunInputsStillAvailable` answers from the platform's own deletion and
-- expiry columns and never probes object storage, which makes it an OPTIMISTIC
-- upper bound: column present, object gone, and the comparison screen offers a
-- re-run that will fail when it fetches the file. The same gap opened on the
-- download side the moment there was something to download — a download page
-- offering bytes that are no longer in the bucket.
--
-- The fix is a periodic reconciler and deliberately NOT a probe on the read path:
-- one HEAD per dataset per comparison buys a round trip for an answer that is
-- already sitting in a column, and it would be bought on every read forever.
--
-- Why a table and not an in-process map — the same reason 0021 gives for the
-- sandbox reconciler: the threshold is "still missing two consecutive rounds",
-- the job is a River leader-only periodic job, and leadership moves between
-- processes. A map would reset the count exactly when the sweep restarts.
--
-- Two consecutive rounds, and not one, because the consequence of acting is
-- marking user content as gone. An object store that answers 404 for an object
-- being written, or during a transient fault, must not cost somebody their file.
--
-- No workspace_id: this is the platform's own reconciliation state about its own
-- storage, it takes no user input, and it is never read on a user's behalf. The
-- resource it names is workspace-scoped and every write it performs goes through
-- a workspace-scoped statement (iron rule 3).
CREATE TABLE object_reconcile_sightings (
    -- Which table the row lives in. Two kinds today and both for the same reason:
    -- a row that claims an object exists.
    resource_kind text NOT NULL CHECK (resource_kind IN ('dataset', 'artifact')),
    resource_id   uuid NOT NULL,
    -- Recorded so an operator reading this table can go and look, without having
    -- to join back to a row that the sweep may already have marked.
    object_key    text NOT NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    -- Consecutive rounds, exactly as 0021 counts them: a round that finds the
    -- object deletes the row, so a reappearance starts over at 1. That is what
    -- makes `rounds >= 2` mean "missing two rounds running" and not "missing
    -- twice at some point".
    rounds        integer NOT NULL DEFAULT 1 CHECK (rounds > 0),
    PRIMARY KEY (resource_kind, resource_id)
);

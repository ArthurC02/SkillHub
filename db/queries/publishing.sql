-- name: CreatePublisher :one
INSERT INTO publishers (workspace_id, name)
VALUES (@workspace_id, @name)
RETURNING *;

-- name: GetPublisherByWorkspace :one
SELECT * FROM publishers WHERE workspace_id = @workspace_id;

-- name: LockPublisherByWorkspace :one
SELECT * FROM publishers WHERE workspace_id = @workspace_id FOR UPDATE;

-- name: GetPublicationForSkill :one
SELECT p.*, pb.name AS publisher_name
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE pb.workspace_id = @workspace_id AND p.skill_id = @skill_id;

-- name: LockPublicationForSkill :one
SELECT p.*
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE pb.workspace_id = @workspace_id AND p.skill_id = @skill_id
FOR UPDATE OF p;

-- name: CreatePublication :one
INSERT INTO publications (publisher_id, name, skill_id, status)
SELECT pb.id, @name, @skill_id, @status
FROM publishers pb
WHERE pb.workspace_id = @workspace_id
RETURNING *;

-- name: SetPublicationStatus :execrows
UPDATE publications p SET status = @status, status_changed_at = now()
FROM publishers pb
WHERE p.id = @id
  AND p.status <> @status
  AND pb.id = p.publisher_id
  AND pb.workspace_id = @workspace_id;

-- name: InsertPublicationRelease :one
INSERT INTO publication_releases (publication_id, skill_version_id, version_number, content_hash,
                                  findings, rights_attested, released_by)
SELECT p.id, @skill_version_id, @version_number, @content_hash, @findings, @rights_attested, @released_by
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE p.id = @publication_id AND pb.workspace_id = @workspace_id
RETURNING *;

-- name: GetPublicPublication :one
SELECT p.*, pb.name AS publisher_name, pb.workspace_id AS publisher_workspace_id
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE pb.name = @publisher_name AND p.name = @name;

-- name: ListPublicationReleases :many
SELECT * FROM publication_releases
WHERE publication_id = @publication_id
ORDER BY released_at DESC, id DESC;

-- name: PurgeWorkspacePublications :execrows
DELETE FROM publications p
USING publishers pb
WHERE pb.id = p.publisher_id AND pb.workspace_id = @workspace_id;

-- name: EnsureBundle :exec
INSERT INTO bundles (workspace_id, name)
VALUES (@workspace_id, @name)
ON CONFLICT (workspace_id, name) DO NOTHING;

-- name: LockBundle :one
SELECT * FROM bundles WHERE workspace_id = @workspace_id AND name = @name FOR UPDATE;

-- name: CreateBundleVersion :one
INSERT INTO bundle_versions (bundle_id, version, description, content_hash, created_by)
SELECT b.id, @version, @description, @content_hash, @created_by
FROM bundles b
WHERE b.id = @bundle_id AND b.workspace_id = @workspace_id
RETURNING *;

-- name: InsertBundleMember :exec
INSERT INTO bundle_members (bundle_version_id, skill_id, skill_version_id, version_number,
                            manifest_name, content_hash, position)
SELECT bv.id, @skill_id, @skill_version_id, @version_number, @manifest_name, @content_hash, @position
FROM bundle_versions bv
JOIN bundles b ON b.id = bv.bundle_id
WHERE bv.id = @bundle_version_id AND b.workspace_id = @workspace_id;

-- name: ListWorkspaceBundleVersions :many
SELECT b.id AS bundle_id, b.name AS bundle_name, bv.*
FROM bundles b
JOIN bundle_versions bv ON bv.bundle_id = b.id
WHERE b.workspace_id = @workspace_id
ORDER BY b.name, bv.created_at DESC, bv.id DESC;

-- name: GetBundleVersion :one
SELECT b.name AS bundle_name, bv.*
FROM bundles b
JOIN bundle_versions bv ON bv.bundle_id = b.id
WHERE b.workspace_id = @workspace_id AND b.name = @bundle_name AND bv.version = @version;

-- name: GetNewestBundleVersion :one
SELECT b.name AS bundle_name, bv.*
FROM bundles b
JOIN bundle_versions bv ON bv.bundle_id = b.id
WHERE b.workspace_id = @workspace_id AND b.name = @bundle_name
ORDER BY bv.created_at DESC, bv.id DESC
LIMIT 1;

-- name: ListBundleMembers :many
SELECT * FROM bundle_members
WHERE bundle_version_id = ANY(@bundle_version_ids::uuid[])
ORDER BY bundle_version_id, position;

-- name: ListSkillVersionsInBundles :many
SELECT DISTINCT skill_version_id FROM bundle_members WHERE skill_version_id = ANY(@version_ids::uuid[]);

-- name: GetPublicationForBundle :one
SELECT p.*, pb.name AS publisher_name
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE pb.workspace_id = @workspace_id AND p.bundle_id = @bundle_id;

-- name: LockPublicationForBundle :one
SELECT p.*
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE pb.workspace_id = @workspace_id AND p.bundle_id = @bundle_id
FOR UPDATE OF p;

-- name: CreateBundlePublication :one
INSERT INTO publications (publisher_id, name, bundle_id, status)
SELECT pb.id, @name, @bundle_id, @status
FROM publishers pb
WHERE pb.workspace_id = @workspace_id
RETURNING *;

-- name: InsertBundleRelease :one
INSERT INTO publication_releases (publication_id, bundle_version_id, content_hash,
                                  findings, rights_attested, released_by)
SELECT p.id, @bundle_version_id, @content_hash, @findings, @rights_attested, @released_by
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE p.id = @publication_id AND pb.workspace_id = @workspace_id
RETURNING *;

-- name: ListReleasedBundleVersions :many
SELECT b.name AS bundle_name, bv.*
FROM bundle_versions bv
JOIN bundles b ON b.id = bv.bundle_id
WHERE bv.id = ANY(@bundle_version_ids::uuid[]);

-- name: PurgeWorkspaceBundles :execrows
DELETE FROM bundles WHERE workspace_id = @workspace_id;

-- name: ListExposureStates :many
SELECT p.id AS publication_id, p.name, p.status, p.skill_id,
       pb.name AS publisher_name, pb.workspace_id AS publisher_workspace_id,
       r.id AS release_id, r.skill_version_id, r.version_number, r.content_hash, r.released_at,
       coalesce(rv.sequence, 0)::integer AS sequence, rv.release_id AS reviewed_release_id,
       coalesce(rv.decision, '')::text AS decision,
       coalesce(rv.snapshot_digest, '')::text AS reviewed_snapshot_digest
FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
JOIN LATERAL (
    SELECT * FROM publication_releases pr WHERE pr.publication_id = p.id
    ORDER BY pr.released_at DESC, pr.id DESC LIMIT 1
) r ON true
LEFT JOIN LATERAL (
    SELECT * FROM exposure_reviews er WHERE er.publication_id = p.id
    ORDER BY er.sequence DESC LIMIT 1
) rv ON true
WHERE p.skill_id IS NOT NULL
  AND (sqlc.narg(publication_id)::uuid IS NULL OR p.id = sqlc.narg(publication_id))
ORDER BY r.released_at, p.id;

-- name: LockPublicationByAddress :one
SELECT p.id FROM publications p
JOIN publishers pb ON pb.id = p.publisher_id
WHERE pb.name = @publisher_name AND p.name = @name
FOR UPDATE OF p;

-- name: InsertExposureReview :one
INSERT INTO exposure_reviews (publication_id, sequence, release_id, content_hash, snapshot_digest,
                              decision, reason, reviewer_user_id)
VALUES (@publication_id, @sequence, @release_id, @content_hash, @snapshot_digest,
        @decision, @reason, @reviewer_user_id)
RETURNING *;

-- name: ListExposureReviews :many
SELECT * FROM exposure_reviews WHERE publication_id = @publication_id ORDER BY sequence DESC;

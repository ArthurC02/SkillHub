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

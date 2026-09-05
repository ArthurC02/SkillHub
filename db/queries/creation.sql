-- name: CreateCreationSession :one
INSERT INTO creation_sessions(id,workspace_id,state,revision,snapshot,expires_at)
VALUES($1,$2,$3,1,$4,$5) RETURNING *;

-- name: GetCreationSession :one
SELECT * FROM creation_sessions WHERE id=$1 AND workspace_id=$2;

-- name: LockCreationSession :one
SELECT * FROM creation_sessions WHERE id=$1 AND workspace_id=$2 FOR UPDATE;

-- name: ListCreationSessions :many
SELECT * FROM creation_sessions WHERE workspace_id=$1 AND expires_at > now()
ORDER BY updated_at DESC LIMIT 50;

-- name: AdvanceCreationSession :one
UPDATE creation_sessions SET state=sqlc.arg(state),snapshot=sqlc.arg(snapshot),
 revision=revision+1,updated_at=now()
WHERE id=sqlc.arg(id) AND workspace_id=sqlc.arg(workspace_id)
 AND revision=sqlc.arg(expected_revision) RETURNING *;

-- name: AppendCreationEvent :exec
INSERT INTO creation_session_events(session_id,workspace_id,revision,event_type,snapshot)
VALUES($1,$2,$3,$4,$5);

-- name: InsertCreationReceipt :one
INSERT INTO creation_receipts(id,session_id,workspace_id,kind,status,expected_revision,request_hash,result)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: GetCreationReceipt :one
SELECT * FROM creation_receipts WHERE id=$1 AND session_id=$2 AND workspace_id=$3;

-- name: ClaimCreationReceipt :one
UPDATE creation_receipts SET status='running'
WHERE id=$1 AND session_id=$2 AND workspace_id=$3 AND status='queued' RETURNING *;

-- name: FinishCreationReceipt :execrows
UPDATE creation_receipts SET status=sqlc.arg(status),result=sqlc.arg(result),
 usage=sqlc.arg(usage),finished_at=now()
WHERE id=sqlc.arg(id) AND session_id=sqlc.arg(session_id) AND workspace_id=sqlc.arg(workspace_id);

-- name: PurgeCreationWorkspace :exec
DELETE FROM creation_sessions WHERE workspace_id=$1;

-- name: DeleteExpiredCreationSessions :execrows
DELETE FROM creation_sessions WHERE expires_at <= now();

-- name: ListStalledCreationSessions :many
SELECT * FROM creation_sessions
WHERE state IN ('working', 'queued') AND updated_at < $1
ORDER BY updated_at LIMIT 100;

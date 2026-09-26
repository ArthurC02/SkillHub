-- name: EnsureCreditAccount :exec
INSERT INTO credit_accounts (user_id) VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;

-- name: GetCreditBalance :one
SELECT * FROM credit_accounts WHERE user_id = $1;

-- name: AdjustCreditBalance :one
UPDATE credit_accounts
SET balance_credits = balance_credits + sqlc.arg(delta_credits), updated_at = now()
WHERE user_id = sqlc.arg(user_id)
RETURNING balance_credits;

-- name: InsertCreditEntry :one
INSERT INTO credit_entries (
    user_id, kind, delta_credits, usd_micros, markup_bps, model, prompt_version,
    ref_type, ref_id, cost_event_id, estimated, idempotency_key
) VALUES (
    sqlc.arg(user_id), sqlc.arg(kind), sqlc.arg(delta_credits), sqlc.arg(usd_micros),
    sqlc.arg(markup_bps), sqlc.arg(model), sqlc.arg(prompt_version), sqlc.arg(ref_type),
    sqlc.arg(ref_id), sqlc.arg(cost_event_id), sqlc.arg(estimated), sqlc.arg(idempotency_key)
)
ON CONFLICT ON CONSTRAINT credit_entries_idempotency_key_key DO NOTHING
RETURNING *;

-- name: ListRecentCreditEntries :many
SELECT * FROM credit_entries
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: SumCreditEntriesByDay :many
SELECT (created_at AT TIME ZONE 'UTC')::date AS day, kind,
       count(*)::bigint AS entries, sum(delta_credits)::bigint AS delta_credits
FROM credit_entries
WHERE created_at >= @since::timestamptz
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: SumCreditBalances :one
SELECT coalesce(sum(balance_credits), 0)::bigint AS balance_total FROM credit_accounts;

-- name: ListOwnCreditEntries :many
SELECT e.id, e.kind, e.delta_credits, e.estimated, e.created_at, e.ref_type, e.ref_id,
       c.kind AS spent_on
FROM credit_entries e
LEFT JOIN cost_events c ON c.id = e.cost_event_id
WHERE e.user_id = @user_id
  AND (sqlc.narg('before_at')::timestamptz IS NULL
       OR (e.created_at, e.id) < (sqlc.narg('before_at')::timestamptz, sqlc.narg('before_id')::uuid))
ORDER BY e.created_at DESC, e.id DESC
LIMIT @row_limit;

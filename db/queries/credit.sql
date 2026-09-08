-- name: EnsureCreditAccount :exec
-- Idempotent existence upsert (not a ledger write): called once at signup,
-- and defensively before the first debit for accounts predating this
-- migration. Never touches balance_credits if the row already exists.
INSERT INTO credit_accounts (user_id) VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;

-- name: GetCreditBalance :one
SELECT * FROM credit_accounts WHERE user_id = $1;

-- name: AdjustCreditBalance :one
-- Applied in the same transaction as the InsertCreditEntry it accounts for,
-- after the insert succeeds (a 23505 on the entry's idempotency_key means the
-- caller already applied this delta on a previous attempt — skip this call
-- rather than double-apply). The row lock this UPDATE takes serializes
-- concurrent debits on one account; the CHECK on credit_accounts enforces
-- the -50 debt floor as the last backstop.
UPDATE credit_accounts
SET balance_credits = balance_credits + sqlc.arg(delta_credits), updated_at = now()
WHERE user_id = sqlc.arg(user_id)
RETURNING balance_credits;

-- name: InsertCreditEntry :one
-- Plain insert, not ON CONFLICT: the codebase's idempotency convention
-- (registry.go, evidence/service.go) is catching pgErr.Code=="23505" on the
-- unique idempotency_key and treating it as "already applied", not upserting.
INSERT INTO credit_entries (
    user_id, kind, delta_credits, usd_micros, markup_bps, model, prompt_version,
    ref_type, ref_id, cost_event_id, estimated, idempotency_key
) VALUES (
    sqlc.arg(user_id), sqlc.arg(kind), sqlc.arg(delta_credits), sqlc.arg(usd_micros),
    sqlc.arg(markup_bps), sqlc.arg(model), sqlc.arg(prompt_version), sqlc.arg(ref_type),
    sqlc.arg(ref_id), sqlc.arg(cost_event_id), sqlc.arg(estimated), sqlc.arg(idempotency_key)
) RETURNING *;

-- name: ListRecentCreditEntries :many
-- Ledger view for a user (balance reconciliation, support, self-service
-- history). Newest first.
SELECT * FROM credit_entries
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: SumCreditEntries :one
-- Reconciliation: what credit_accounts.balance_credits should equal, summed
-- from the entries actually still retained. Diverges from the live balance
-- once entries older than the retention window have been purged — that is
-- the known, accepted cost of PurgeExpiredCreditEntries (05 "保存期限與刪除").
SELECT coalesce(sum(delta_credits), 0)::bigint AS total_delta_credits
FROM credit_entries WHERE user_id = $1;

-- name: PurgeExpiredCreditEntries :execrows
-- Retention sweep (05 "保存期限與刪除"), same shape as
-- audit.PurgeExpired/DeleteExpiredAuditEvents: caller runs this inside a
-- transaction with `SET LOCAL skillhub.purge = 'on'` set first, which is what
-- lets this DELETE past credit_entries_immutable (0060's enforce_immutable
-- trigger). credit_accounts.balance_credits is unaffected — it was already
-- updated when the entry was written, not derived from surviving rows.
DELETE FROM credit_entries WHERE created_at < $1;

-- name: PurgeUserCreditEntries :execrows
-- Account deletion, not retention (see PurgeUserCostEvents). The account row
-- itself goes with the user's FK cascade; these entries are addressed by
-- user_id and have to be named explicitly.
-- Same purge guard: `SET LOCAL skillhub.purge = 'on'` before this DELETE.
DELETE FROM credit_entries WHERE user_id = $1;

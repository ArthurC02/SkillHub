-- name: CreateUser :one
INSERT INTO users (email, display_name)
VALUES ($1, $2)
RETURNING *;

-- GetUser and GetUserByEmail were removed on 2026-08-25: neither had a caller,
-- in product code or in a test. ADR-035's ratchet works by comparing a query's
-- declared owner against the context that calls it, so a query nobody calls is a
-- declaration nothing has ever checked -- and identity resolves an account
-- through user_identities (ADR-020), not by email, so an email lookup was never
-- going to be the way in. Restore them the day something needs them, with the
-- caller in the same commit.

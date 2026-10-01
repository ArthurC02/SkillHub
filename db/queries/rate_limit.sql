-- name: TakeRateLimitToken :one
INSERT INTO rate_limit_buckets AS b (key, tokens, at)
VALUES (sqlc.arg(key), sqlc.arg(burst)::float8 - 1, now())
ON CONFLICT (key) DO UPDATE SET
    allowed = least(sqlc.arg(burst)::float8, b.tokens + extract(epoch FROM now() - b.at)::float8 * sqlc.arg(rate)::float8) >= 1,
    tokens = least(sqlc.arg(burst)::float8, b.tokens + extract(epoch FROM now() - b.at)::float8 * sqlc.arg(rate)::float8)
        - (least(sqlc.arg(burst)::float8, b.tokens + extract(epoch FROM now() - b.at)::float8 * sqlc.arg(rate)::float8) >= 1)::int,
    at = now()
RETURNING allowed, tokens;

-- name: SweepRateLimitBuckets :execrows
DELETE FROM rate_limit_buckets WHERE at < now() - sqlc.arg(idle)::interval;

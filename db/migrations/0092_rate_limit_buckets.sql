CREATE TABLE rate_limit_buckets (
    key text PRIMARY KEY,
    tokens double precision NOT NULL,
    at timestamptz NOT NULL,
    allowed boolean NOT NULL DEFAULT true
);

ALTER TABLE rate_limit_buckets SET UNLOGGED;

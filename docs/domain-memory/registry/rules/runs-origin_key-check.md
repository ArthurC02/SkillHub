---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0122_run_origin_key.sql#L1-2"
    excerpt_sha256: "sha256:80197268fac9cb019a32aff7be324d15ae4345af267f451e613d32a91061466e"
---
runs keeps check runs.origin_key: origin_key text CONSTRAINT runs_origin_key_shape CHECK (origin_key IS NULL OR (btrim(origin_key) <> '' AND length(origin_key) <= 200))

# Relations
- Contexts: [run](/contexts/run)

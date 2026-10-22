---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0129_run_cost_settled.sql#L9-14"
    excerpt_sha256: "sha256:978a8028315a038503e7b743fbaf7b35e12547af6799687e0439959359664b8d"
---
runs keeps trigger runs_terminal_immutable: CREATE TRIGGER runs_terminal_immutable BEFORE UPDATE OR DELETE ON runs FOR EACH ROW WHEN (OLD.status IN ('succeeded', 'failed', 'cancelled', 'timed_out')) EXECUTE FUNCTION enforce_immutable('cleanup_status', 'cleanup_at', 'cleanup_attempte…

# Relations
- Contexts: [run](/contexts/run)

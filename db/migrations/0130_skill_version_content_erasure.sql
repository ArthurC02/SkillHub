ALTER TABLE skill_versions
    ADD COLUMN content_erased_at timestamptz,
    ADD CONSTRAINT skill_versions_erased_content_cleared
        CHECK (content_erased_at IS NULL OR (manifest = '{}'::jsonb AND source_id IS NULL));

COMMENT ON COLUMN skill_versions.content_erased_at IS
    'Set once when the owner''s account purge erased this version''s content (manifest, import source, package object) because only the owner''s own Runs still referenced it. The row stays as a tombstone so those Runs keep their identity.';

DROP TRIGGER skill_versions_immutable ON skill_versions;
CREATE TRIGGER skill_versions_immutable
    BEFORE DELETE ON skill_versions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();
CREATE TRIGGER skill_versions_frozen
    BEFORE UPDATE ON skill_versions
    FOR EACH ROW
    WHEN (OLD.content_erased_at IS NOT NULL OR NEW.content_erased_at IS NULL)
    EXECUTE FUNCTION enforce_immutable();
CREATE TRIGGER skill_versions_content_erasure
    BEFORE UPDATE ON skill_versions
    FOR EACH ROW
    WHEN (OLD.content_erased_at IS NULL AND NEW.content_erased_at IS NOT NULL)
    EXECUTE FUNCTION enforce_immutable('manifest', 'source_id', 'content_erased_at');

GRANT UPDATE ON skill_versions, skills TO skillhub_purge;
GRANT DELETE ON search_documents TO skillhub_purge;

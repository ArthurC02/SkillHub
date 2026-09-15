ALTER TABLE evaluation_suggestions
    ADD CONSTRAINT evaluation_suggestions_id_workspace_key UNIQUE (id, workspace_id);

ALTER TABLE skill_versions
    ADD CONSTRAINT skill_versions_id_workspace_key UNIQUE (id, workspace_id);

CREATE TABLE evaluation_suggestion_applications (
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    suggestion_id uuid NOT NULL,
    skill_version_id uuid NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (suggestion_id, skill_version_id),
    FOREIGN KEY (suggestion_id, workspace_id)
        REFERENCES evaluation_suggestions (id, workspace_id),
    FOREIGN KEY (skill_version_id, workspace_id)
        REFERENCES skill_versions (id, workspace_id) ON DELETE CASCADE
);

CREATE INDEX evaluation_suggestion_applications_version_idx
    ON evaluation_suggestion_applications (workspace_id, skill_version_id, suggestion_id);

INSERT INTO evaluation_suggestion_applications (workspace_id, suggestion_id, skill_version_id)
SELECT workspace_id, id, applied_skill_version_id
FROM evaluation_suggestions
WHERE applied_skill_version_id IS NOT NULL
ON CONFLICT (suggestion_id, skill_version_id) DO NOTHING;

CREATE TRIGGER evaluation_suggestion_applications_immutable
    BEFORE UPDATE OR DELETE ON evaluation_suggestion_applications
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

CREATE TRIGGER account_purge_insert_fence
    BEFORE INSERT ON evaluation_suggestion_applications
    FOR EACH ROW EXECUTE FUNCTION fence_workspace_write_during_account_purge();

GRANT SELECT, DELETE ON evaluation_suggestion_applications TO skillhub_purge;

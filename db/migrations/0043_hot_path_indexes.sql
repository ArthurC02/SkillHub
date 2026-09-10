CREATE INDEX analytics_events_workspace_id_idx ON analytics_events (workspace_id)
    WHERE workspace_id IS NOT NULL;

CREATE INDEX feedback_reports_workspace_id_idx ON feedback_reports (workspace_id)
    WHERE workspace_id IS NOT NULL;

CREATE INDEX skills_curated_version_id_idx ON skills (curated_version_id)
    WHERE curated_version_id IS NOT NULL;

CREATE INDEX skills_forked_from_version_id_idx ON skills (forked_from_version_id)
    WHERE forked_from_version_id IS NOT NULL;

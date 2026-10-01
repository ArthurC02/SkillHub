DROP INDEX run_status_transitions_workspace_id_idx;

CREATE INDEX run_status_transitions_workspace_status_idx
    ON run_status_transitions (workspace_id, to_status, occurred_at);

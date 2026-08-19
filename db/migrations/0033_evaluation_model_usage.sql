-- One evaluation may make two separately billable model calls. Keep their
-- gateway readings append-only and separate; suggestion spend must never be
-- folded into the verdict row's historical judge-only cost.
ALTER TABLE evaluations
    ADD CONSTRAINT evaluations_id_workspace_key UNIQUE (id, workspace_id);

CREATE TABLE evaluation_model_usage (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    evaluation_id     uuid NOT NULL,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id),
    operation         text NOT NULL CHECK (operation IN ('judge', 'suggest')),
    model             text NOT NULL,
    prompt_version    text NOT NULL,
    prompt_tokens     bigint NOT NULL CHECK (prompt_tokens >= 0),
    completion_tokens bigint NOT NULL CHECK (completion_tokens >= 0),
    cost_usd          numeric(12, 6) CHECK (cost_usd IS NULL OR cost_usd >= 0),
    cost_source       text CHECK (cost_source IN ('gateway', 'estimated')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT evaluation_model_usage_cost_needs_source
        CHECK ((cost_usd IS NULL) = (cost_source IS NULL)),
    CONSTRAINT evaluation_model_usage_once_per_operation
        UNIQUE (evaluation_id, operation),
    CONSTRAINT evaluation_model_usage_evaluation_workspace_fk
        FOREIGN KEY (evaluation_id, workspace_id)
        REFERENCES evaluations (id, workspace_id)
);

CREATE INDEX evaluation_model_usage_evaluation_id_idx
    ON evaluation_model_usage (evaluation_id, created_at);

CREATE TRIGGER evaluation_model_usage_immutable
BEFORE UPDATE OR DELETE ON evaluation_model_usage
FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

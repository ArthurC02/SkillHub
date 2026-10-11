CREATE TABLE evaluation_graders (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid REFERENCES workspaces (id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 100),
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX evaluation_graders_platform_name_key ON evaluation_graders (name) WHERE workspace_id IS NULL;
CREATE UNIQUE INDEX evaluation_graders_workspace_name_key ON evaluation_graders (workspace_id, name) WHERE workspace_id IS NOT NULL;

COMMENT ON TABLE evaluation_graders IS
    'A named way of judging a criterion with the judge model. No workspace means operators defined it for every workspace.';

CREATE TABLE evaluation_grader_versions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    grader_id    uuid NOT NULL REFERENCES evaluation_graders (id) ON DELETE CASCADE,
    version      integer NOT NULL CHECK (version >= 1),
    instructions text NOT NULL CHECK (btrim(instructions) <> '' AND length(instructions) <= 4000),
    reason       text NOT NULL CHECK (btrim(reason) <> ''),
    created_by   uuid REFERENCES users (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (grader_id, version)
);

CREATE TRIGGER evaluation_grader_versions_immutable
    BEFORE UPDATE OR DELETE ON evaluation_grader_versions
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

COMMENT ON TABLE evaluation_grader_versions IS
    'The judging instructions of a grader, one immutable row per version; a check names the version it uses.';

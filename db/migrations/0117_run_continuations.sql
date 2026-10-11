CREATE TABLE run_continuations (
    run_id           uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL,
    continues_run_id uuid NOT NULL UNIQUE,
    questions        jsonb NOT NULL CHECK (jsonb_typeof(questions) = 'array' AND jsonb_array_length(questions) > 0),
    answers          jsonb NOT NULL CHECK (jsonb_typeof(answers) = 'array'),
    answered_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT run_continuations_answer_every_question
        CHECK (jsonb_array_length(answers) = jsonb_array_length(questions)),
    CONSTRAINT run_continuations_not_itself CHECK (run_id <> continues_run_id),
    CONSTRAINT run_continuations_run_fkey
        FOREIGN KEY (run_id, workspace_id) REFERENCES runs (id, workspace_id) ON DELETE CASCADE,
    CONSTRAINT run_continuations_continues_fkey
        FOREIGN KEY (continues_run_id, workspace_id) REFERENCES runs (id, workspace_id) ON DELETE CASCADE
);

CREATE TRIGGER run_continuations_immutable
    BEFORE UPDATE ON run_continuations
    FOR EACH ROW EXECUTE FUNCTION enforce_immutable();

COMMENT ON TABLE run_continuations IS
    'A run started from the questions another run ended with and the person''s answers to them; one continuation per asking run.';

CREATE TABLE run_settings (
    singleton           boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    continuation_rounds integer NOT NULL CHECK (continuation_rounds BETWEEN 1 AND 10),
    reason              text NOT NULL CHECK (btrim(reason) <> ''),
    set_by              uuid REFERENCES users (id) ON DELETE SET NULL,
    set_at              timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE run_settings IS
    'Operator-set run policy. An absent row means every setting is at its default.';

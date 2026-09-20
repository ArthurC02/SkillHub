CREATE TABLE model_call_budgets (
    kind    text PRIMARY KEY,
    seconds integer NOT NULL CHECK (seconds > 0),
    reason  text NOT NULL CHECK (btrim(reason) <> ''),
    set_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    set_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE model_call_budgets IS
    'An operator-set per-call ceiling for one model endpoint (02:OPS-009). An absent row means the compiled default. Which kinds exist, and how far below the compiled deadline a value may sit, are decided in Go; this table stores a number and who set it.';

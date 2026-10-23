CREATE TABLE run_model_budget_setting (
    singleton     boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    budget_micros bigint NOT NULL CHECK (budget_micros BETWEEN 50000 AND 5000000),
    reason        text NOT NULL CHECK (btrim(reason) <> ''),
    set_by        uuid REFERENCES users (id) ON DELETE SET NULL,
    set_at        timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE run_model_budget_setting IS
    'The operator-set model spend ceiling of one run. An absent row means the deployment default applies; each run keeps the ceiling it was created under in its policy snapshot.';

COMMENT ON COLUMN run_model_budget_setting.singleton IS
    'Always true; it limits the table to a single row.';

COMMENT ON COLUMN run_model_budget_setting.budget_micros IS
    'The model spend ceiling of one run in millionths of a US dollar, from 0.05 to 5 dollars.';

COMMENT ON COLUMN run_model_budget_setting.reason IS
    'Why the operator set this value. Never blank.';

COMMENT ON COLUMN run_model_budget_setting.set_by IS
    'The operator who set the value. NULL if that account has since been removed.';

COMMENT ON COLUMN run_model_budget_setting.set_at IS
    'When the value was set.';

CREATE TABLE run_rate_limit_setting (
    singleton  boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    tpm_limit  integer NOT NULL CHECK (tpm_limit BETWEEN 10000 AND 2000000),
    reason     text NOT NULL CHECK (btrim(reason) <> ''),
    set_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    set_at     timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE run_rate_limit_setting IS
    'The operator-set tokens-per-minute limit of a run''s model gateway key. An absent row means the deployment default applies; every key issued from then on reads the current value.';

COMMENT ON COLUMN run_rate_limit_setting.singleton IS
    'Always true; it limits the table to a single row.';

COMMENT ON COLUMN run_rate_limit_setting.tpm_limit IS
    'How many model tokens a run attempt''s gateway key may use per minute, from 10,000 to 2,000,000.';

COMMENT ON COLUMN run_rate_limit_setting.reason IS
    'Why the operator set this value. Never blank.';

COMMENT ON COLUMN run_rate_limit_setting.set_by IS
    'The operator who set the value. NULL if that account has since been removed.';

COMMENT ON COLUMN run_rate_limit_setting.set_at IS
    'When the value was set.';

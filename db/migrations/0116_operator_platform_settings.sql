ALTER TABLE platform_agents
    ADD COLUMN daily_spend_cap_override_micros bigint CHECK (daily_spend_cap_override_micros > 0);

COMMENT ON COLUMN platform_agents.daily_spend_cap_override_micros IS
    'An operator-set daily spend cap that replaces the one defined in code; null means the defined cap applies.';

CREATE TABLE evaluation_settings (
    singleton   boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    judge_panel boolean NOT NULL,
    reason      text NOT NULL CHECK (btrim(reason) <> ''),
    set_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    set_at      timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE evaluation_settings IS
    'Operator-set evaluation policy. An absent row means every setting is at its default: one judge, no panel.';

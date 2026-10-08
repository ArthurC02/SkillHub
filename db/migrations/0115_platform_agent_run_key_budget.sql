ALTER TABLE platform_agent_runs ADD COLUMN key_budget_micros bigint CHECK (key_budget_micros > 0);

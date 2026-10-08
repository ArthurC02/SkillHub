ALTER TABLE audit_events
    ADD COLUMN actor_agent_id uuid REFERENCES platform_agents (id),
    ADD CONSTRAINT audit_events_one_actor CHECK (num_nonnulls(actor_user_id, actor_agent_id) <= 1);

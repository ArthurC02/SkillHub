COMMENT ON TABLE analytics_events IS
    'One anonymous product-usage event (search, detail view, session start, download start) with coarse query shape and no query text; feeds the beta funnel measurements. Partitioned by month and expired after the configured analytics retention.';

COMMENT ON TABLE artifacts IS
    'One stored output file of a run or a download package, with its object key, content hash, size and scan status. The row stays after the object expires; maintenance marks it purged once the retention window passes.';

COMMENT ON TABLE audit_events IS
    'One security-relevant action by an actor on a resource, with structured metadata. Immutable: a trail that could be rewritten would not be a trail. Expires after the configured audit retention.';

COMMENT ON TABLE bundle_members IS
    'One skill version pinned inside a bundle version, with its manifest name, content hash and position. Immutable: changing it would rewrite the set someone already installed.';

COMMENT ON TABLE bundle_versions IS
    'One released, pinned list of skill versions under a bundle, identified by a version label and a content hash. Immutable: upgrading a member creates a new bundle version instead of editing this one.';

COMMENT ON TABLE bundles IS
    'A named, workspace-owned collection of skills that is versioned and downloaded as one unit.';

COMMENT ON TABLE cost_events IS
    'One paid model call made by the platform, with model, token counts, cost in micro-dollars and whether the cost was measured or estimated. Immutable and never deleted; recomputing statistics creates new windows instead of changing events.';

COMMENT ON TABLE cost_session_summaries IS
    'Per creation session roll-up of cost: total micro-dollars, number of steps and whether any step was estimated. Kept permanently, even across account purge.';

COMMENT ON TABLE cost_statistics IS
    'One percentile summary (median, 90th, 95th, maximum cost) of a kind of model call over a time window, used to decide whether a paid call may start and what it is expected to cost. Derived data recomputed daily.';

COMMENT ON TABLE creation_receipts IS
    'One command or attempt received by a creation session, with the revision it expected, a hash of the request and its result and usage; lets a retried request return the same outcome. Deleted with its session.';

COMMENT ON TABLE creation_session_events IS
    'One revision of a creation session, storing the full session snapshot after an event so the session history can be replayed. Deleted with its session.';

COMMENT ON TABLE creation_sessions IS
    'One interactive skill-authoring conversation in a workspace, holding its current state, revision counter and snapshot. Expires at expires_at.';

COMMENT ON TABLE credit_accounts IS
    'One row per user holding the current credit balance; the balance is the running total of the user''s credit entries. Never deleted, not even by account purge.';

COMMENT ON TABLE credit_entries IS
    'One change to a user''s credit balance: a debit for model use, a grant, a top-up or an adjustment, each with an idempotency key. Immutable: corrections are new entries. Never deleted, not even by account purge.';

COMMENT ON TABLE database_size_samples IS
    'One measurement of the total database size per calendar day, used to watch storage growth. One row is kept per day.';

COMMENT ON TABLE dataset_object_cleanup_intents IS
    'A pending promise to delete a dataset object that was uploaded but never recorded in a manifest, due after not_before. The row is deleted once the object is gone.';

COMMENT ON TABLE datasets IS
    'One uploaded input file attached to a test case, with its object key, size and content hash. The row stays after the object expires; maintenance marks it purged once the retention window passes.';

COMMENT ON TABLE download_artifact_members IS
    'One skill version that a downloadable plugin package was built from, with its position. Immutable: repackaging creates new rows.';

COMMENT ON TABLE download_artifacts IS
    'The package-specific details of a downloadable artifact: which skill version, target, profile and packager version, manifest hash and whether test cases are included. Immutable: the produced bytes and their hashes never change, repackaging is a new row. The row stays after the object expires.';

COMMENT ON TABLE download_object_cleanup_intents IS
    'A pending promise to delete a package object that was uploaded but never recorded in a manifest, due after not_before. The row is deleted once the object is gone.';

COMMENT ON TABLE download_records IS
    'One fact that a user downloaded a package at a given time. Immutable: it records what happened and is not editable state. Deleted with the owner by account purge.';

COMMENT ON TABLE evaluation_model_usage IS
    'The model, prompt version, token counts and cost used by one judging or suggestion step of an evaluation. Immutable: re-evaluating appends new rows.';

COMMENT ON TABLE evaluation_suggestion_applications IS
    'The link between an accepted evaluation suggestion and the new skill version created from it. Immutable and append-only: later applications never overwrite an earlier source.';

COMMENT ON TABLE evaluation_suggestions IS
    'One proposed improvement produced from an evaluation, with the problem, evidence, target file, proposed change, expected impact and the user''s decision. Accepting a suggestion creates a new skill version rather than editing in place.';

COMMENT ON TABLE evaluations IS
    'The verdict on one run: overall result, per-criterion results, summary, judge model and the user''s feedback on the verdict.';

COMMENT ON TABLE exposure_reviews IS
    'One operator decision to approve or revoke the public exposure of a publication release, with the reason and the content digest that was reviewed. Immutable: a revocation is a new row and earlier judgements stay on record.';

COMMENT ON TABLE feedback_reports IS
    'One piece of feedback a user submitted, either a blocking issue or a signal of need, with the page it came from. Expires after the configured feedback retention.';

COMMENT ON TABLE generation_leases IS
    'A short-lived per-workspace lock that allows only one skill generation at a time; a row exists only while the lease is held or not yet expired.';

COMMENT ON TABLE maintenance_job_runs IS
    'One row per scheduled maintenance job recording its period and when it last succeeded, so overdue jobs can be detected. Jobs no longer scheduled are forgotten.';

COMMENT ON TABLE object_reconcile_sightings IS
    'A dataset or artifact whose object was seen missing or stray, with how many reconciliation rounds it has persisted; an object is acted on only after repeated sightings. Deleted when the sighting clears.';

COMMENT ON TABLE outbox_events IS
    'One domain event written in the same transaction as the state change that caused it, waiting to be published to consumers. Published events expire after seven days; dead letters stay.';

COMMENT ON TABLE platform_agent_brake IS
    'A single row that exists only while the operator has engaged the emergency brake that stops all platform agents, with who engaged it and why.';

COMMENT ON TABLE platform_agent_finding_events IS
    'One step in the life of a platform agent finding (opened, seen again, acknowledged, resolved and so on), made either by an agent run or by an operator with a note. Archivable with the finding.';

COMMENT ON TABLE platform_agent_findings IS
    'One deduplicated problem reported by a platform agent, with its title, cited evidence, status, assignee and how often it has been seen.';

COMMENT ON TABLE platform_agent_proposals IS
    'One action a platform agent proposes for an operator to approve, with its risk tier, reason, cited evidence, preview and lifecycle from proposed to executed or expired.';

COMMENT ON TABLE platform_agent_runs IS
    'One execution of a platform agent, with its status, start and finish time and the reason it stopped or failed.';

COMMENT ON TABLE platform_agent_steps IS
    'One tool call made during a platform agent run, with its arguments, result, model and token and cost usage.';

COMMENT ON TABLE platform_agents IS
    'One platform agent defined in code, with its purpose, model role, daily spend cap, allowed tools and actions, owner and whether it is enabled. One row per agent.';

COMMENT ON TABLE publication_releases IS
    'The fact that a particular skill version was published under a name, with its content hash, scan findings, rights attestation and releaser. Immutable: a new release is a new row, because rewriting it would rewrite what others have seen.';

COMMENT ON TABLE publications IS
    'A skill made publicly available under a publisher and a name, with whether it is currently published or delisted.';

COMMENT ON TABLE publishers IS
    'The public identity of a workspace under which it publishes skills; one publisher per workspace with a unique name.';

COMMENT ON TABLE rate_limit_buckets IS
    'The token-bucket state of one rate-limit key, used to throttle requests across API processes. Idle buckets are swept by the limiter.';

COMMENT ON TABLE reconciler_orphan_sightings IS
    'A provider run that exists at the execution provider but has no matching platform run, with how many reconciliation rounds it has persisted before it is cleaned up. Deleted when the orphan clears.';

COMMENT ON TABLE run_artifact_upload_intents IS
    'A pending promise to delete an object uploaded by a run attempt if no output manifest ever records it, due after not_before. The row is deleted once the object is settled.';

COMMENT ON TABLE run_attempts IS
    'One execution attempt of a run at a provider, with its provider run id, timing and error class and message. Archivable with the run.';

COMMENT ON TABLE run_permission_confirmations IS
    'The record that a user confirmed the permissions a skill version requests before it was run with a given test case. Kept permanently.';

COMMENT ON TABLE run_snapshots IS
    'The runtime and policy configuration that applied to one run, kept apart from the hot run row. Archivable with the run.';

COMMENT ON TABLE run_status_transitions IS
    'One change of a run from one status to another, with the reason and time. Immutable: it is the history of what happened to the run, and rewriting it would rewrite that history. Archivable.';

COMMENT ON TABLE runs IS
    'One execution of a skill version against a frozen test case snapshot, with its status, attempt count, provider and cleanup state. The permanent identity of a run; its child rows are archivable.';

COMMENT ON TABLE search_documents IS
    'The searchable projection of one skill: name, summary, full-text vector and embedding used by catalog search. Derived data kept in step with the skill.';

COMMENT ON TABLE sessions IS
    'One login session of a user, identified by a hashed token and valid until expires_at.';

COMMENT ON TABLE skill_runtime_compatibility IS
    'The measured result of whether a skill version activated and ran on a given runtime image, and which run measured it.';

COMMENT ON TABLE skill_sources IS
    'Where an imported skill package came from (a git repository at a ref, or an upload) with the content hash of what was fetched.';

COMMENT ON TABLE skill_versions IS
    'One numbered version of a skill: its package location, content hash, parsed manifest and declared license. Immutable: adopting a suggestion creates a new version instead of overwriting this one.';

COMMENT ON TABLE skills IS
    'One skill owned by a workspace: its name, summary, category, curation tier, licensing and redistribution gates and, for a fork, the skill it came from. deleted_at marks a deletion that waits for the grace period before purge.';

COMMENT ON TABLE test_case_snapshots IS
    'The frozen copy of a test case''s prompt, criteria and dataset references taken when a run starts. Immutable: changing it would make the inputs of historical runs lie. Archivable.';

COMMENT ON TABLE test_cases IS
    'One test scenario for a skill: a user prompt and acceptance criteria that runs are judged against.';

COMMENT ON TABLE trace_events IS
    'One execution fact emitted during a run, with its sequence number, type, source and payload. Immutable and partitioned by month; expires after the configured trace retention by dropping partitions.';

COMMENT ON TABLE user_identities IS
    'The link between a user and an external login identity, unique per provider and provider user id.';

COMMENT ON TABLE users IS
    'One person with an account, with email and display name; deleted_at marks a closed account. The row stays after account purge so credit records keep their reference.';

COMMENT ON TABLE workspaces IS
    'The tenant boundary that owns skills, test cases, runs and other user data; each is owned by one user.';

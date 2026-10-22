COMMENT ON COLUMN artifacts.workspace_id IS
    'The workspace that owns the file; every read of it is scoped to this workspace.';

COMMENT ON COLUMN artifacts.kind IS
    'What the file is: run_output is a file produced by a run, download_package is a packaged skill built for the owner to download.';

COMMENT ON COLUMN artifacts.file_name IS
    'The name the file is offered under when it is downloaded.';

COMMENT ON COLUMN artifacts.content_type IS
    'The media type the file is served with.';

COMMENT ON COLUMN artifacts.size_bytes IS
    'The size of the stored file in bytes.';

COMMENT ON COLUMN artifacts.content_hash IS
    'The hash of the file''s bytes, used to detect corruption and to recognise identical content.';

COMMENT ON COLUMN artifacts.object_key IS
    'Where the file''s bytes live in object storage.';

COMMENT ON COLUMN artifacts.scan_status IS
    'Whether the file may be served: quarantined until it has been scanned, available once it passed, rejected when the scan refused it.';

COMMENT ON COLUMN artifacts.expires_at IS
    'When retention removes the stored object; after that the row stays but the bytes are gone.';

COMMENT ON COLUMN artifacts.deleted_at IS
    'When the owner deleted the file, which hides it from every listing and download. NULL while it still exists for the owner.';

COMMENT ON COLUMN artifacts.reconcile_checked_at IS
    'When the object reconciler last compared this row with object storage. NULL if never; the least recently checked rows are taken first so every row gets a turn.';

COMMENT ON COLUMN artifacts.retention_attempted_at IS
    'When retention last tried to remove this file''s object. NULL if never; the least recently attempted rows go first so one failing row cannot starve the others.';

COMMENT ON COLUMN audit_events.actor_user_id IS
    'The user who performed the action. NULL when an agent or the system acted; at most one of this and actor_agent_id is set.';

COMMENT ON COLUMN audit_events.workspace_id IS
    'The workspace the action happened in. NULL for actions that belong to no workspace.';

COMMENT ON COLUMN audit_events.resource_type IS
    'The kind of thing the action was done to; resource_id says which one.';

COMMENT ON COLUMN audit_events.resource_id IS
    'The id of the resource the action was done to. NULL when the action has no single target.';

COMMENT ON COLUMN audit_events.metadata IS
    'Details of the action as JSON, such as what changed; callers must not put secrets here. Empty object when there are none.';

COMMENT ON COLUMN audit_events.actor_agent_id IS
    'The platform agent that performed the action. NULL when a user or the system acted.';

COMMENT ON COLUMN database_size_samples.sampled_on IS
    'The calendar day this sample stands for; sampling again on the same day overwrites the row.';

COMMENT ON COLUMN database_size_samples.database_bytes IS
    'The total size of the database in bytes when it was sampled.';

COMMENT ON COLUMN database_size_samples.sampled_at IS
    'When the measurement was actually taken.';

COMMENT ON COLUMN dispatch_halts.provider IS
    'The execution provider whose dispatch is stopped. The empty string means the whole provider pool.';

COMMENT ON COLUMN dispatch_halts.source IS
    'What stopped dispatch: p1_incident is a halt an operator declared, orphan_threshold is one raised automatically because too many provider runs had no matching platform run.';

COMMENT ON COLUMN dispatch_halts.reason IS
    'Why dispatch was stopped, in the operator''s words. Never blank.';

COMMENT ON COLUMN dispatch_halts.declared_by IS
    'The user who declared the halt. NULL when it was raised automatically or the user has since been removed.';

COMMENT ON COLUMN dispatch_halts.declared_at IS
    'When the halt took effect.';

COMMENT ON COLUMN dispatch_halts.clear_rounds IS
    'How many consecutive reconciliation rounds have found the triggering condition gone; an automatic halt is lifted only after enough clear rounds.';

COMMENT ON COLUMN dispatch_halts.lifted_at IS
    'When the halt was lifted. NULL while it is still in force.';

COMMENT ON COLUMN dispatch_halts.lifted_by IS
    'The user who lifted the halt. NULL while it is in force, or when it was lifted automatically.';

COMMENT ON COLUMN dispatch_halts.lift_reason IS
    'Why the halt was lifted. Set exactly when lifted_at is set.';

COMMENT ON COLUMN dispatch_halts.last_clear_round_at IS
    'When the most recent clear round was counted. NULL until the first one.';

COMMENT ON COLUMN maintenance_job_runs.job IS
    'The name of the scheduled maintenance job.';

COMMENT ON COLUMN maintenance_job_runs.period_seconds IS
    'How often the job is expected to succeed, in seconds; a job that has not succeeded within this period is overdue.';

COMMENT ON COLUMN maintenance_job_runs.registered_at IS
    'When the job was first registered; for a job that has never succeeded, the overdue clock runs from here.';

COMMENT ON COLUMN maintenance_job_runs.succeeded_at IS
    'When the job last finished successfully. NULL if it never has.';

COMMENT ON COLUMN model_call_budgets.kind IS
    'The model endpoint the ceiling applies to: one of enrich-skill, generate-skill, analyze-intent, match-reasons, suggest-criteria, judge-run or suggest-improvements.';

COMMENT ON COLUMN model_call_budgets.seconds IS
    'The longest a call to this endpoint may run, in seconds: at least 1 and at most the calling code''s deadline minus a five-second safety margin.';

COMMENT ON COLUMN model_call_budgets.reason IS
    'Why the operator set this value. Never blank.';

COMMENT ON COLUMN model_call_budgets.set_by IS
    'The operator who set the value. NULL if that account has since been removed.';

COMMENT ON COLUMN model_call_budgets.set_at IS
    'When the value was set.';

COMMENT ON COLUMN object_reconcile_sightings.resource_kind IS
    'Which kind of record the object belongs to: dataset or artifact.';

COMMENT ON COLUMN object_reconcile_sightings.resource_id IS
    'The id of the dataset or artifact whose object was seen missing or stray.';

COMMENT ON COLUMN object_reconcile_sightings.object_key IS
    'The object storage key that was seen missing or stray.';

COMMENT ON COLUMN object_reconcile_sightings.first_seen_at IS
    'When the problem was first seen.';

COMMENT ON COLUMN object_reconcile_sightings.last_seen_at IS
    'When the problem was most recently seen.';

COMMENT ON COLUMN object_reconcile_sightings.rounds IS
    'How many reconciliation rounds in a row have seen the problem; action is taken only after repeated sightings.';

COMMENT ON COLUMN outbox_events.event_id IS
    'The unique id of the event; consumers use it to recognise a redelivery of an event they have already handled.';

COMMENT ON COLUMN outbox_events.event_type IS
    'What happened, as a dotted name such as run.succeeded, drawn from a fixed catalogue of event types.';

COMMENT ON COLUMN outbox_events.event_version IS
    'The version of the payload layout for this event type.';

COMMENT ON COLUMN outbox_events.occurred_at IS
    'When the state change that caused the event was committed.';

COMMENT ON COLUMN outbox_events.correlation_id IS
    'An id shared by every event and piece of work that came from the same originating request, so they can be traced together.';

COMMENT ON COLUMN outbox_events.causation_id IS
    'The id of the event or command that directly caused this one. NULL for an event that starts a chain.';

COMMENT ON COLUMN outbox_events.workspace_id IS
    'The workspace the event belongs to. NULL for events outside any workspace.';

COMMENT ON COLUMN outbox_events.aggregate_type IS
    'The kind of record the event is about: run, evaluation or skill.';

COMMENT ON COLUMN outbox_events.aggregate_id IS
    'The id of the run, evaluation or skill the event is about.';

COMMENT ON COLUMN outbox_events.payload IS
    'The body of the event as consumers receive it.';

COMMENT ON COLUMN outbox_events.published_at IS
    'When the event was delivered to consumers. NULL while it is still waiting.';

COMMENT ON COLUMN outbox_events.delivery_attempts IS
    'How many deliveries of this event have failed so far; at the configured maximum the event is dead-lettered.';

COMMENT ON COLUMN outbox_events.dead_lettered_at IS
    'When delivery was given up after too many failures. A dead-lettered event is no longer retried and is not purged.';

COMMENT ON COLUMN outbox_events.next_delivery_at IS
    'The earliest moment the event may be claimed again: set while a dispatcher holds the event and after a failed delivery to space out retries; NULL when it is ready now.';

COMMENT ON COLUMN rate_limit_buckets.key IS
    'What is being throttled: the route, then a bar, then the caller, as account:<id> for a signed-in caller or addr:<address> otherwise.';

COMMENT ON COLUMN rate_limit_buckets.tokens IS
    'The tokens left in the bucket as of the at time, as a fraction; it refills at the limiter''s rate up to its burst size and each admitted request spends one.';

COMMENT ON COLUMN rate_limit_buckets.at IS
    'When the bucket was last refilled and evaluated.';

COMMENT ON COLUMN rate_limit_buckets.allowed IS
    'Whether the most recent request on this key was admitted.';

COMMENT ON COLUMN reconciler_orphan_sightings.provider IS
    'The execution provider where the unmatched run exists.';

COMMENT ON COLUMN reconciler_orphan_sightings.provider_run_id IS
    'The id the provider gave the run that has no matching platform run.';

COMMENT ON COLUMN reconciler_orphan_sightings.first_seen_at IS
    'When the orphan was first seen.';

COMMENT ON COLUMN reconciler_orphan_sightings.last_seen_at IS
    'When the orphan was most recently seen.';

COMMENT ON COLUMN reconciler_orphan_sightings.rounds IS
    'How many reconciliation rounds in a row have seen the orphan; it is cleaned up only after repeated sightings.';

COMMENT ON COLUMN run_artifact_upload_intents.run_attempt_id IS
    'The run attempt whose uploaded output archive this promise covers; one per attempt.';

COMMENT ON COLUMN run_artifact_upload_intents.workspace_id IS
    'The workspace that owns the run attempt.';

COMMENT ON COLUMN run_artifact_upload_intents.object_key IS
    'The object storage key of the output archive the attempt may have uploaded.';

COMMENT ON COLUMN run_artifact_upload_intents.not_before IS
    'The earliest time the object may be deleted, set safely after the attempt''s upload grants have expired.';

COMMENT ON COLUMN run_artifact_upload_intents.attempted_at IS
    'When cleanup last tried to settle this object. NULL if never; the least recently attempted rows go first.';

COMMENT ON COLUMN run_attempts.run_id IS
    'The run this attempt belongs to.';

COMMENT ON COLUMN run_attempts.workspace_id IS
    'The workspace that owns the run.';

COMMENT ON COLUMN run_attempts.attempt_number IS
    'The position of this attempt within its run, counting from 1; unique per run.';

COMMENT ON COLUMN run_attempts.provider IS
    'The execution provider this attempt was sent to.';

COMMENT ON COLUMN run_attempts.provider_run_id IS
    'The id the provider gave this attempt. NULL until the provider has accepted it.';

COMMENT ON COLUMN run_attempts.error_class IS
    'Why the attempt failed: provision, execution, cleanup, capability_mismatch, budget_exhausted, provider_lost, timeout, cancelled or resource_limit. NULL when it did not fail.';

COMMENT ON COLUMN run_attempts.error_message IS
    'Diagnostic detail the provider or platform gave when the attempt failed, kept for operators; the API shows fixed wording per error_class instead of this text. NULL when the attempt did not fail or gave no detail.';

COMMENT ON COLUMN run_attempts.started_at IS
    'When the provider began executing the attempt. NULL if it never started.';

COMMENT ON COLUMN run_attempts.finished_at IS
    'When the attempt ended. NULL while it is still going.';

COMMENT ON COLUMN run_attempts.object_grants_expire_at IS
    'When the short-lived object storage grants handed to this attempt stop working; a far-future placeholder until they are issued.';

COMMENT ON COLUMN run_attempts.object_grants_state IS
    'Whether object grants were issued for this attempt: unissued, recorded once issued, closed when the attempt or run ended before any grant was issued (its expiry is then already past), or legacy_unknown for rows older than this tracking.';

COMMENT ON COLUMN run_attempts.provider_unreachable_since IS
    'When the provider first stopped answering for this attempt. NULL while it is reachable.';

COMMENT ON COLUMN run_continuations.run_id IS
    'The run that was started from the answers; it is the continuation.';

COMMENT ON COLUMN run_continuations.workspace_id IS
    'The workspace that owns both runs.';

COMMENT ON COLUMN run_continuations.continues_run_id IS
    'The earlier run that ended with questions; each run can be continued at most once.';

COMMENT ON COLUMN run_continuations.questions IS
    'The questions the earlier run ended with, as a non-empty JSON array.';

COMMENT ON COLUMN run_continuations.answers IS
    'The answers given to those questions, as a JSON array with one entry per question in the same order.';

COMMENT ON COLUMN run_continuations.answered_by IS
    'The user who submitted the answers, or on whose behalf a simulated user answered when answered_as_simulated_user is true. NULL if that account has since been removed.';

COMMENT ON COLUMN run_permission_confirmations.workspace_id IS
    'The workspace in which the user confirmed.';

COMMENT ON COLUMN run_permission_confirmations.skill_version_id IS
    'The skill version whose requested permissions were confirmed.';

COMMENT ON COLUMN run_permission_confirmations.test_case_id IS
    'The test case the version was confirmed to run with.';

COMMENT ON COLUMN run_permission_confirmations.summary_hash IS
    'The hash of the permission summary the user was shown, so a change in what the skill asks for no longer matches an earlier confirmation.';

COMMENT ON COLUMN run_permission_confirmations.confirmed_by IS
    'The user who confirmed the permissions.';

COMMENT ON COLUMN run_permission_confirmations.confirmed_at IS
    'When the user confirmed.';

COMMENT ON COLUMN run_settings.singleton IS
    'Always true; it limits the table to a single row.';

COMMENT ON COLUMN run_settings.continuation_rounds IS
    'How many times in a row a run may be continued with answers to its questions, from 1 to 10.';

COMMENT ON COLUMN run_settings.reason IS
    'Why the operator set this value. Never blank.';

COMMENT ON COLUMN run_settings.set_by IS
    'The operator who set the value. NULL if that account has since been removed.';

COMMENT ON COLUMN run_settings.set_at IS
    'When the value was set.';

COMMENT ON COLUMN run_snapshots.run_id IS
    'The run the snapshots describe; one row per run.';

COMMENT ON COLUMN run_snapshots.workspace_id IS
    'The workspace that owns the run.';

COMMENT ON COLUMN run_snapshots.runtime_snapshot IS
    'The runtime image and configuration the run executes with; it can be completed while the run is open and is frozen once the run has finished.';

COMMENT ON COLUMN run_snapshots.policy_snapshot IS
    'The limits and permissions that applied to the run, fixed when the row is created.';

COMMENT ON COLUMN run_status_transitions.run_id IS
    'The run whose status changed.';

COMMENT ON COLUMN run_status_transitions.workspace_id IS
    'The workspace that owns the run.';

COMMENT ON COLUMN run_status_transitions.from_status IS
    'The status the run left. NULL for the first row, which records the run being created.';

COMMENT ON COLUMN run_status_transitions.to_status IS
    'The status the run entered.';

COMMENT ON COLUMN run_status_transitions.reason IS
    'Why the change happened, in words shown to people. NULL when no reason was given.';

COMMENT ON COLUMN run_status_transitions.occurred_at IS
    'When the status changed.';

COMMENT ON COLUMN run_status_transitions.run_attempt_id IS
    'The attempt that was current when the status changed. NULL for changes not tied to one attempt.';

COMMENT ON COLUMN runs.workspace_id IS
    'The workspace that owns the run.';

COMMENT ON COLUMN runs.skill_version_id IS
    'The exact, immutable skill version that was run.';

COMMENT ON COLUMN runs.test_case_snapshot_id IS
    'The frozen copy of the test case the version was run against.';

COMMENT ON COLUMN runs.status IS
    'Where the run is in its lifecycle: queued, provisioning, preparing, running, evaluating, then one of the final states succeeded, failed, cancelled or timed_out.';

COMMENT ON COLUMN runs.status_reason IS
    'The reason given for the run''s most recent status change, in words shown to people and sometimes relayed from the provider. Replaced on each change; NULL when none was given.';

COMMENT ON COLUMN runs.provider IS
    'The execution provider chosen for the run.';

COMMENT ON COLUMN runs.cleanup_status IS
    'How far releasing the run''s execution resources has got: pending, cleaning_up, cleaned or failed.';

COMMENT ON COLUMN runs.cleanup_at IS
    'When cleanup finished, whether it ended cleaned or failed. NULL until then; moving to cleaning_up does not set it.';

COMMENT ON COLUMN runs.started_at IS
    'When the run first entered the running status. NULL until then, and for runs that ended before they ran.';

COMMENT ON COLUMN runs.finished_at IS
    'When the run reached a final status. Set exactly when the status is final.';

COMMENT ON COLUMN runs.cancel_requested_at IS
    'When the user asked to cancel the run. NULL if they have not.';

COMMENT ON COLUMN runs.failure_class IS
    'Why a failed run failed: provider_error, workload_error, timeout, cancelled, capability_mismatch, policy_refused, platform_error or resource_limit. NULL when the run did not fail.';

COMMENT ON COLUMN runs.supervision_checked_at IS
    'When the supervisor last examined this unfinished run. NULL if never; the least recently checked runs go first.';

COMMENT ON COLUMN runs.cleanup_attempted_at IS
    'When cleanup of this finished run last ran. NULL if never; the least recently attempted runs go first.';

COMMENT ON COLUMN runs.artifacts_truncated IS
    'True when the provider reported that collecting the run''s output was cut short, either because files were dropped or because a file was cut off, so the stored output is incomplete.';

COMMENT ON COLUMN runs.activity_updated_at IS
    'When the run last changed status or received a cancel request; the workspace activity feed orders runs by it.';

COMMENT ON COLUMN runs.cost_settled_at IS
    'When the run''s final model spend was read from the model gateway and recorded. NULL while still waiting for it.';

COMMENT ON COLUMN runs.spend_rechecked_at IS
    'When the platform last asked the model gateway for this run''s spend while waiting for it to settle. NULL if it has not yet.';

COMMENT ON COLUMN search_documents.skill_id IS
    'The skill this searchable projection describes; one row per skill.';

COMMENT ON COLUMN search_documents.workspace_id IS
    'The workspace that owns the skill.';

COMMENT ON COLUMN search_documents.name IS
    'The skill''s name as catalog search matches and shows it.';

COMMENT ON COLUMN search_documents.summary IS
    'The skill''s own summary as written by its author. Empty when it has none.';

COMMENT ON COLUMN search_documents.tsv IS
    'The full-text vector over name and summary, weighting the name above the summary. Derived by the database.';

COMMENT ON COLUMN search_documents.embedding IS
    'The semantic vector of the skill''s text used for similarity search. NULL until it has been computed.';

COMMENT ON COLUMN search_documents.enriched_summary IS
    'A summary written by the enrichment model to make the skill easier to find. Empty until enrichment has run.';

COMMENT ON COLUMN search_documents.task_examples IS
    'Example tasks the skill can do, written by the enrichment model. Empty until enrichment has run.';

COMMENT ON COLUMN search_documents.enrichment_status IS
    'pending while the enrichment model has not yet processed the skill, enriched once it has.';

COMMENT ON COLUMN search_documents.enrichment_model IS
    'The model that produced the enrichment. NULL until enriched.';

COMMENT ON COLUMN search_documents.enrichment_prompt_version IS
    'The version of the enrichment prompt used, so older enrichments can be redone when the prompt changes. NULL until enriched.';

COMMENT ON COLUMN search_documents.tags IS
    'The tags the enrichment model chose for the skill, as a JSON object with arrays named inputs, outputs, tools and dependencies. NULL until enriched.';

COMMENT ON COLUMN search_documents.limitations IS
    'What the skill cannot do or should not be used for, written by enrichment. Empty until enriched.';

COMMENT ON COLUMN search_documents.scan IS
    'A summary of the static safety scan of the skill''s package as JSON: the number of warnings and the distinct finding codes. NULL when no scan result is available.';

COMMENT ON COLUMN search_documents.enrichment_attempted_at IS
    'When enrichment last tried this skill. NULL if never; the least recently attempted skills go first.';

COMMENT ON COLUMN search_documents.bigram IS
    'Full-text vector of lexical tokens from the name, summary, enriched summary, task examples and tags: lowercased Latin words plus overlapping two-character pieces of Chinese characters, which the word-based vector cannot segment. Used for the lexical leg of search.';

COMMENT ON COLUMN search_documents.generated IS
    'True when the skill was generated by the platform rather than contributed as an existing package.';

COMMENT ON COLUMN search_documents.category IS
    'The catalog category the skill is filed under. NULL when it has none.';

COMMENT ON COLUMN search_documents.category_source IS
    'Who chose the category: curated by a catalog curator, or owner by the skill''s owner. NULL exactly when category is NULL.';

COMMENT ON COLUMN search_documents.latest_version_id IS
    'The newest version of the skill. NULL when the skill has no version.';

COMMENT ON COLUMN search_documents.verified_at IS
    'When the newest version was created. Catalog search treats a non-NULL value as the skill having a validated version and uses it to rank recency. NULL when the skill has no version.';

COMMENT ON COLUMN search_documents.latest_package_object_key IS
    'Where the newest version''s package is stored. NULL exactly when latest_version_id is NULL.';

COMMENT ON COLUMN search_documents.curated_version_id IS
    'The version a curator approved. NULL unless the skill is in the curated tier.';

COMMENT ON COLUMN search_documents.agent_capability IS
    'Whether the agent noticed and activated the skill on the runtime image it was measured on: activated, not_activated, or unverified when it has not been measured.';

COMMENT ON COLUMN search_documents.agent_runtime IS
    'How the skill''s scripts ran on the runtime image: native (as written), transpiled (after the platform adapted them), failed, or unverified when it has not been measured.';

COMMENT ON COLUMN search_documents.agent_runtime_image IS
    'The runtime image the measurement was made on. Empty when unverified.';

COMMENT ON COLUMN search_documents.agent_measured_at IS
    'When the runtime compatibility was measured. NULL when it never was.';

COMMENT ON COLUMN search_documents.listable IS
    'Whether catalog search may show the skill: true once it has been enriched or has an embedding.';

COMMENT ON COLUMN search_documents.has_script IS
    'True when the scan found a script file or embedded script in the package. NULL when there is no scan result to tell.';

COMMENT ON COLUMN search_documents.latest_source_path IS
    'The folder inside the source repository that the newest version came from. Empty when it is at the root.';

COMMENT ON COLUMN search_documents.exposure_digest IS
    'A hash of everything a reader sees in the listing, so a change to the shown content can be told from an unchanged one. Derived by the database.';

COMMENT ON COLUMN search_documents.curated IS
    'True when the version a curator approved is the newest version. Derived by the database.';

COMMENT ON COLUMN trace_events.workspace_id IS
    'The workspace that owns the run.';

COMMENT ON COLUMN trace_events.run_id IS
    'The run the event was emitted during.';

COMMENT ON COLUMN trace_events.seq IS
    'The producer''s own running number for its events, so a gap shows that an event was lost.';

COMMENT ON COLUMN trace_events.event_type IS
    'What kind of fact this is: skill_activation, resource_read, tool_call, mcp_call, script_log, agent_output, error, usage, run_lifecycle, evaluation_started or evaluation_completed.';

COMMENT ON COLUMN trace_events.source IS
    'Who emitted the event: sandbox, orchestrator or llm_service.';

COMMENT ON COLUMN trace_events.status IS
    'How the step ended: ok, error, skipped, cancelled or timed_out. NULL when the event has no outcome.';

COMMENT ON COLUMN trace_events.payload IS
    'The event''s content as JSON, with secrets masked.';

COMMENT ON COLUMN trace_events.payload_object_key IS
    'Where the payload is stored in object storage when it was too large to keep in the row. NULL otherwise.';

COMMENT ON COLUMN trace_events.event_id IS
    'The id the producer gave the event, so sending the same event twice stores it once.';

COMMENT ON COLUMN trace_events.attempt IS
    'The run attempt, counting from 1, during which the event was emitted.';

COMMENT ON COLUMN trace_events.schema_version IS
    'The version of the event layout the producer used.';

COMMENT ON COLUMN trace_events.masked IS
    'Always true: the payload was masked of secrets before it was stored.';

COMMENT ON COLUMN trace_events.masked_fields IS
    'The names of the payload fields that were masked.';

COMMENT ON COLUMN trace_events.late IS
    'True when the event arrived after its run had already reached a final status.';

COMMENT ON COLUMN trace_events.ingest_seq IS
    'The order in which the platform received events, assigned per run, so a reader can resume from the last number it saw.';

COMMENT ON COLUMN artifacts.run_id IS
    'The run that produced the file; NULL for a package built for download, which belongs to no run.';

COMMENT ON COLUMN audit_events.action IS
    'What was done, as a dotted name from the platform''s fixed audit vocabulary, such as auth.login, skill.import, skill.takedown, account.purged, run.create or run.transition.';

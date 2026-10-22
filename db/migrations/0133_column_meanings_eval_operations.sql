COMMENT ON COLUMN dataset_object_cleanup_intents.workspace_id IS
    'The workspace that uploaded the object; scopes the cleanup to its owner.';

COMMENT ON COLUMN dataset_object_cleanup_intents.object_key IS
    'The storage key of the uploaded dataset object that has no manifest row; unique, so each object has at most one pending cleanup.';

COMMENT ON COLUMN dataset_object_cleanup_intents.not_before IS
    'The earliest time the object may be deleted; set when the upload slot is reserved, one hour ahead, so an upload still in flight is not cleaned up. The row is deleted once the manifest row is recorded.';

COMMENT ON COLUMN dataset_object_cleanup_intents.attempted_at IS
    'When maintenance last tried to delete the object; null means never tried. Due rows are worked least recently attempted first so a stuck row cannot starve the rest.';

COMMENT ON COLUMN datasets.workspace_id IS
    'The workspace that owns the dataset; every read is scoped by it.';

COMMENT ON COLUMN datasets.test_case_id IS
    'The test case this input file is attached to.';

COMMENT ON COLUMN datasets.file_name IS
    'The original name of the uploaded file as the user gave it.';

COMMENT ON COLUMN datasets.content_type IS
    'The media type recorded for the uploaded file.';

COMMENT ON COLUMN datasets.size_bytes IS
    'The size of the uploaded file in bytes; never negative.';

COMMENT ON COLUMN datasets.content_hash IS
    'The hash of the file bytes, used to pin exactly which content a test case snapshot refers to.';

COMMENT ON COLUMN datasets.object_key IS
    'The storage key under which the file bytes are kept in object storage.';

COMMENT ON COLUMN datasets.expires_at IS
    'When the retention window ends; after this time maintenance deletes the stored object and marks the row purged.';

COMMENT ON COLUMN datasets.deleted_at IS
    'When the dataset was deleted, by its owner, by deleting its test case, or by maintenance once the stored object was purged or found lost; null while live. A deleted dataset is no longer offered to runs and its object becomes due for cleanup.';

COMMENT ON COLUMN datasets.reconcile_checked_at IS
    'When maintenance last picked this row up to verify that its stored object still exists; set when the row is claimed, null means never. Not picked up again until the claim lease has passed, least recently picked first.';

COMMENT ON COLUMN datasets.retention_attempted_at IS
    'When maintenance last picked this row up to remove the stored object of an expired or deleted dataset; set when the row is claimed, null means never. Not retried until the claim lease has passed.';

COMMENT ON COLUMN datasets.purged_at IS
    'When the stored object was confirmed gone; null while the object may still exist. The row itself is kept.';

COMMENT ON COLUMN evaluation_batch_items.batch_id IS
    'The batch this trial belongs to.';

COMMENT ON COLUMN evaluation_batch_items.workspace_id IS
    'The workspace of the batch; the batch, test case and run must all belong to it.';

COMMENT ON COLUMN evaluation_batch_items.test_case_id IS
    'The test case this trial runs.';

COMMENT ON COLUMN evaluation_batch_items.position IS
    'The zero-based order of the test case within the batch, which decides the order runs are started in.';

COMMENT ON COLUMN evaluation_batch_items.trial IS
    'Which repetition of the test case this is, counting from 1 up to the batch''s trials.';

COMMENT ON COLUMN evaluation_batch_items.summary_hash IS
    'The hash of the run permission summary the user confirmed for this skill version and test case when the batch was launched; the run is only started if the permission summary still hashes the same.';

COMMENT ON COLUMN evaluation_batch_items.run_id IS
    'The run started for this trial; null until the batch starts it, and unique so one run serves at most one trial.';

COMMENT ON COLUMN evaluation_batches.workspace_id IS
    'The workspace that launched the batch; the skill and version must belong to it.';

COMMENT ON COLUMN evaluation_batches.skill_id IS
    'The skill whose test cases are evaluated.';

COMMENT ON COLUMN evaluation_batches.skill_version_id IS
    'The skill version every run in the batch uses; fixed at launch so results compare one version.';

COMMENT ON COLUMN evaluation_batches.trials IS
    'How many times each test case is run, from 1 to 10.';

COMMENT ON COLUMN evaluation_batches.status IS
    'launching: items exist but not all runs are started yet. launched: every run has been started. paused: held back, with stop_reason saying why. stopped: ended early, with stop_reason saying why.';

COMMENT ON COLUMN evaluation_batches.stop_reason IS
    'Why a paused or stopped batch is not continuing; set exactly when status is paused or stopped and null otherwise.';

COMMENT ON COLUMN evaluation_batches.created_by IS
    'The user who launched the batch; null if that account was later removed.';

COMMENT ON COLUMN evaluation_batches.paused_at IS
    'When the batch was paused so no further runs are started; set exactly while status is paused and null otherwise.';

COMMENT ON COLUMN evaluation_grader_versions.grader_id IS
    'The grader these instructions belong to.';

COMMENT ON COLUMN evaluation_grader_versions.version IS
    'The version number of the grader, counting from 1; a check refers to a grader by name and version.';

COMMENT ON COLUMN evaluation_grader_versions.instructions IS
    'The text given to the judge model that says how to decide whether a criterion is met, at most 4000 characters.';

COMMENT ON COLUMN evaluation_grader_versions.reason IS
    'Why this version was written, so a later reader can see what changed and why.';

COMMENT ON COLUMN evaluation_grader_versions.created_by IS
    'The user who wrote this version.';

COMMENT ON COLUMN evaluation_graders.workspace_id IS
    'The workspace that defined the grader; null means operators defined it for every workspace.';

COMMENT ON COLUMN evaluation_graders.name IS
    'The grader''s name, up to 100 characters; unique within its workspace, or among platform graders when there is no workspace.';

COMMENT ON COLUMN evaluation_graders.created_by IS
    'The user who created the grader; null if that account was later removed.';

COMMENT ON COLUMN evaluation_model_usage.evaluation_id IS
    'The evaluation this model call was made for.';

COMMENT ON COLUMN evaluation_model_usage.workspace_id IS
    'The workspace of the evaluation; the evaluation must belong to it.';

COMMENT ON COLUMN evaluation_model_usage.operation IS
    'Which step of the evaluation made the call. judge: deciding the verdict. suggest: writing improvement suggestions. Each evaluation has at most one row per operation.';

COMMENT ON COLUMN evaluation_model_usage.model IS
    'The model that actually served the call, as reported by the gateway.';

COMMENT ON COLUMN evaluation_model_usage.prompt_version IS
    'The version of the prompt sent for this model call; ''unknown'' when the caller did not declare one.';

COMMENT ON COLUMN evaluation_model_usage.prompt_tokens IS
    'The number of input tokens the call consumed.';

COMMENT ON COLUMN evaluation_model_usage.completion_tokens IS
    'The number of output tokens the call produced.';

COMMENT ON COLUMN evaluation_model_usage.cost_usd IS
    'The cost of the call in US dollars; null when the cost is unknown, and then cost_source is null too.';

COMMENT ON COLUMN evaluation_model_usage.cost_source IS
    'gateway: the cost was reported by the model gateway. estimated: the cost was computed from token counts. Null exactly when cost_usd is null.';

COMMENT ON COLUMN evaluation_settings.singleton IS
    'Always true; the primary key makes the table hold at most one row.';

COMMENT ON COLUMN evaluation_settings.judge_panel IS
    'Whether evaluations are judged by a panel of several judges instead of a single judge.';

COMMENT ON COLUMN evaluation_settings.reason IS
    'Why the operator chose this setting.';

COMMENT ON COLUMN evaluation_settings.set_by IS
    'The operator who last changed the setting; null if that account was later removed.';

COMMENT ON COLUMN evaluation_settings.set_at IS
    'When the setting was last changed.';

COMMENT ON COLUMN evaluation_suggestion_applications.workspace_id IS
    'The workspace of both the suggestion and the skill version it produced.';

COMMENT ON COLUMN evaluation_suggestion_applications.suggestion_id IS
    'The accepted suggestion that was applied.';

COMMENT ON COLUMN evaluation_suggestion_applications.skill_version_id IS
    'The new skill version created by applying the suggestion; deleted along with that version.';

COMMENT ON COLUMN evaluation_suggestion_applications.applied_at IS
    'When the suggestion was applied to create the version.';

COMMENT ON COLUMN evaluation_suggestions.evaluation_id IS
    'The evaluation whose result this suggestion was derived from.';

COMMENT ON COLUMN evaluation_suggestions.category IS
    'What the problem is about. skill: the skill''s own content. runtime: the execution environment or its dependencies. tool: the tools the run used. dataset: the test input. mcp is accepted by the constraint but never stored.';

COMMENT ON COLUMN evaluation_suggestions.problem IS
    'What went wrong in the run, in the judge''s words.';

COMMENT ON COLUMN evaluation_suggestions.evidence IS
    'The one observation from the evaluation whose excerpt contains a fragment the suggestion quoted verbatim, with its kind, whether it was available and an excerpt. A suggestion with no matching observation is not stored.';

COMMENT ON COLUMN evaluation_suggestions.target_path IS
    'The file inside the skill package that the change would touch; always a relative path that stays inside the package.';

COMMENT ON COLUMN evaluation_suggestions.proposed_content IS
    'The full new content of the target file; applying the suggestion replaces that file with this text in a new skill version.';

COMMENT ON COLUMN evaluation_suggestions.expected_impact IS
    'What the judge expects to improve if the change is made.';

COMMENT ON COLUMN evaluation_suggestions.decision IS
    'pending: the user has not decided. accepted: the user wants it applied. rejected: the user declined it.';

COMMENT ON COLUMN evaluation_suggestions.decided_at IS
    'When the user accepted or rejected the suggestion; null exactly while the decision is pending.';

COMMENT ON COLUMN evaluation_suggestions.applied_skill_version_id IS
    'The skill version created when the suggestion was applied; null until then and only possible for accepted suggestions.';

COMMENT ON COLUMN evaluations.workspace_id IS
    'The workspace of the run being judged; every read is scoped by it.';

COMMENT ON COLUMN evaluations.run_id IS
    'The run this verdict is about. A run can have several evaluations over time but only one current one.';

COMMENT ON COLUMN evaluations.overall IS
    'The verdict on the run. met: every criterion is satisfied. partially_met: some are. not_met: none that matter are. undetermined: there was not enough evidence to decide.';

COMMENT ON COLUMN evaluations.summary IS
    'A short written explanation of the verdict; null when none was produced.';

COMMENT ON COLUMN evaluations.criterion_results IS
    'One entry per acceptance criterion with its result and the evidence behind it; every evidence item names its kind, whether it was available and an excerpt.';

COMMENT ON COLUMN evaluations.judge_model IS
    'The model that actually gave the verdict. Null when the platform''s own checks decided every criterion or there were no criteria. A failed evaluation keeps the judge model declared at its start, null when no judge is configured.';

COMMENT ON COLUMN evaluations.feedback_helpful IS
    'Whether the user found the verdict helpful; null until they answer. One of the few columns editable after the evaluation completes.';

COMMENT ON COLUMN evaluations.feedback_comment IS
    'The free-text comment the user left about the verdict; null when none. Editable after the evaluation completes.';

COMMENT ON COLUMN evaluations.status IS
    'pending: judging has started but has no result. completed: the verdict is final and the row is frozen apart from feedback. failed: judging could not finish.';

COMMENT ON COLUMN evaluations.judge_prompt_version IS
    'The identifier of the judge prompt that produced the verdict. Null when no model judged; a failed evaluation keeps the version declared at its start.';

COMMENT ON COLUMN evaluations.rubric_version IS
    'The version of the test case rubric the model judged against. Null when the test case has no rubric and when the platform''s own checks completed the evaluation without a model.';

COMMENT ON COLUMN evaluations.evidence_complete IS
    'Whether every observation the verdict depends on could be read; false means the verdict was reached with some evidence missing.';

COMMENT ON COLUMN evaluations.deterministic_findings IS
    'Checks made by plain code rather than the judge model, each with the evidence it found; they are recorded next to the model''s verdict.';

COMMENT ON COLUMN evaluations.cost_usd IS
    'The cost of producing this evaluation in US dollars; null when unknown, and then cost_source is null too.';

COMMENT ON COLUMN evaluations.cost_source IS
    'gateway: the cost was reported by the model gateway. estimated: it was computed from token counts. Null exactly when cost_usd is null.';

COMMENT ON COLUMN evaluations.cost_is_lower_bound IS
    'Always true: nothing sets it, and the gateway''s per-key spend is the authoritative cost, so cost_usd is only what the judge call reported.';

COMMENT ON COLUMN evaluations.evaluated_at IS
    'When judging finished; null while it has not.';

COMMENT ON COLUMN evaluations.superseded_at IS
    'When a newer evaluation of the same run replaced this one; null for the current evaluation, which is the only one a run may have.';

COMMENT ON COLUMN platform_agent_brake.engaged IS
    'Always true; the primary key makes the table hold at most one row, and the row''s presence means the brake is on.';

COMMENT ON COLUMN platform_agent_brake.engaged_by IS
    'The operator who most recently engaged the brake; engaging again replaces it.';

COMMENT ON COLUMN platform_agent_brake.engaged_at IS
    'When the brake was last engaged; engaging again replaces it.';

COMMENT ON COLUMN platform_agent_brake.reason IS
    'Why the operator most recently stopped all platform agents; engaging again replaces it.';

COMMENT ON COLUMN platform_agent_finding_events.finding_id IS
    'The finding this event is part of.';

COMMENT ON COLUMN platform_agent_finding_events.seq IS
    'The position of the event in its finding''s history, counting from 0.';

COMMENT ON COLUMN platform_agent_finding_events.kind IS
    'What happened. opened: first reported. seen: reported again. reopened: came back after being closed. recovered: the problem no longer shows. acknowledged: an operator took it on. resolved: an operator fixed it. dismissed: an operator judged it not a problem.';

COMMENT ON COLUMN platform_agent_finding_events.run_id IS
    'The agent run that made the observation; set when an agent caused the event and null when an operator did.';

COMMENT ON COLUMN platform_agent_finding_events.operator_id IS
    'The operator who made the change; set when a person caused the event and null when an agent run did.';

COMMENT ON COLUMN platform_agent_finding_events.text IS
    'The finding title the agent reported at that sighting; null for recovered events and every operator event.';

COMMENT ON COLUMN platform_agent_finding_events.evidence IS
    'The evidence map the agent attached to that sighting; null for recovered events and every operator event.';

COMMENT ON COLUMN platform_agent_finding_events.note IS
    'The operator''s explanation for their action; required whenever an operator made the event.';

COMMENT ON COLUMN platform_agent_finding_events.occurred_at IS
    'When the event happened.';

COMMENT ON COLUMN platform_agent_findings.agent_id IS
    'The platform agent that reported the problem.';

COMMENT ON COLUMN platform_agent_findings.status IS
    'open: reported and not yet handled. acknowledged: an operator took it on. resolved: an operator fixed it. dismissed: an operator judged it not a problem. recovered: the agent stopped seeing the problem on its own.';

COMMENT ON COLUMN platform_agent_findings.title IS
    'A short description of the problem, refreshed each time the agent sees it again.';

COMMENT ON COLUMN platform_agent_findings.cites IS
    'Values the agent quoted from what its tools returned, at least one. A new sighting that shares any cite with an earlier finding is treated as the same problem. Replaced on every sighting.';

COMMENT ON COLUMN platform_agent_findings.assignee_id IS
    'The operator responsible for the finding; required once it is acknowledged and null before that.';

COMMENT ON COLUMN platform_agent_findings.first_seen_at IS
    'When the agent first reported the problem.';

COMMENT ON COLUMN platform_agent_findings.last_seen_at IS
    'When the agent most recently reported the problem; findings are listed newest sighting first.';

COMMENT ON COLUMN platform_agent_findings.seen_count IS
    'How many times the agent has reported this same problem, starting at 1.';

COMMENT ON COLUMN platform_agent_findings.status_changed_at IS
    'When the status last changed.';

COMMENT ON COLUMN platform_agent_proposals.agent_id IS
    'The platform agent that made the proposal.';

COMMENT ON COLUMN platform_agent_proposals.run_id IS
    'The agent run in which the proposal was made.';

COMMENT ON COLUMN platform_agent_proposals.action IS
    'The name of the maintenance job action the agent wants run, in the form run-<job>; at most one proposal per action can be proposed, approved or running at a time.';

COMMENT ON COLUMN platform_agent_proposals.tier IS
    'How risky the action is. read_only: changes nothing. reversible: changes something that can be undone. destructive: cannot be undone.';

COMMENT ON COLUMN platform_agent_proposals.reason IS
    'Why the agent proposes the action.';

COMMENT ON COLUMN platform_agent_proposals.cites IS
    'Values the agent quoted from what its tools returned to justify the proposal, at least one, checked against those results before it is stored.';

COMMENT ON COLUMN platform_agent_proposals.preview IS
    'How many items each part of the action would affect, as counts with an at-most flag, plus the batch limit when there is one; recorded when the proposal is made and shown to the operator before they decide.';

COMMENT ON COLUMN platform_agent_proposals.status IS
    'proposed: waiting for an operator. approved: accepted and waiting to run. rejected: declined. expired: no decision before expires_at. running: being executed. succeeded: executed successfully. failed: execution failed and outcome says why.';

COMMENT ON COLUMN platform_agent_proposals.proposed_at IS
    'When the agent made the proposal.';

COMMENT ON COLUMN platform_agent_proposals.expires_at IS
    'The time after which an undecided proposal can no longer be approved; always later than proposed_at.';

COMMENT ON COLUMN platform_agent_proposals.decided_by IS
    'The operator who approved or rejected the proposal; null while it is proposed or after it expired unanswered.';

COMMENT ON COLUMN platform_agent_proposals.decided_at IS
    'When the operator decided; null exactly when decided_by is null.';

COMMENT ON COLUMN platform_agent_proposals.decision_note IS
    'The operator''s explanation for approving or rejecting; required whenever a decision was made.';

COMMENT ON COLUMN platform_agent_proposals.started_at IS
    'When execution of the approved action began; null if it never started.';

COMMENT ON COLUMN platform_agent_proposals.finished_at IS
    'When the proposal reached a closing state (rejected, expired, succeeded or failed); null while it is still open or running.';

COMMENT ON COLUMN platform_agent_proposals.outcome IS
    'Why the action failed, or that it was abandoned by a stopped maintenance process; null for any proposal that did not fail.';

COMMENT ON COLUMN platform_agent_runs.agent_id IS
    'The platform agent that was run.';

COMMENT ON COLUMN platform_agent_runs.status IS
    'running: still executing. completed: finished its work. incomplete: ended because it hit a limit (daily spend cap, step limit, token limit or time limit). stopped: halted because the agent brake was engaged or the agent was disabled. failed: ended with an error.';

COMMENT ON COLUMN platform_agent_runs.started_at IS
    'When the run began.';

COMMENT ON COLUMN platform_agent_runs.finished_at IS
    'When the run ended; null exactly while it is running.';

COMMENT ON COLUMN platform_agent_runs.reason IS
    'Why a run that did not complete ended; required for incomplete, stopped and failed runs.';

COMMENT ON COLUMN platform_agent_runs.result IS
    'The structured result the agent returned at the end of the run; also kept on a run that failed because its result did not pass validation. Null on other runs.';

COMMENT ON COLUMN platform_agent_runs.key_budget_micros IS
    'The spending limit, in millionths of a US dollar, of the model key issued for this run; counted as the run''s spend when any of its steps could not be priced. Null when the run ended before a key was issued.';

COMMENT ON COLUMN platform_agent_steps.run_id IS
    'The agent run this tool call belongs to.';

COMMENT ON COLUMN platform_agent_steps.seq IS
    'The position of the call within the run, counting from 0.';

COMMENT ON COLUMN platform_agent_steps.tool IS
    'The name of the tool the agent called.';

COMMENT ON COLUMN platform_agent_steps.arguments IS
    'The arguments the agent passed to the tool, as text.';

COMMENT ON COLUMN platform_agent_steps.result IS
    'What the tool returned to the agent, as text.';

COMMENT ON COLUMN platform_agent_steps.model IS
    'The model that produced this step.';

COMMENT ON COLUMN platform_agent_steps.prompt_tokens IS
    'The number of input tokens this step consumed.';

COMMENT ON COLUMN platform_agent_steps.completion_tokens IS
    'The number of output tokens this step produced.';

COMMENT ON COLUMN platform_agent_steps.usd_micros IS
    'The cost of the step in millionths of a US dollar; null when it could not be priced.';

COMMENT ON COLUMN platform_agents.name IS
    'The agent''s unique name, lowercase letters, digits and hyphens, starting with a letter.';

COMMENT ON COLUMN platform_agents.purpose IS
    'What the agent is for, in one sentence.';

COMMENT ON COLUMN platform_agents.model_role IS
    'The model role alias the gateway resolves to the model this agent uses.';

COMMENT ON COLUMN platform_agents.daily_spend_cap_micros IS
    'The most the agent may spend per day, in millionths of a US dollar, as defined in code; an operator override replaces it.';

COMMENT ON COLUMN platform_agents.tools IS
    'The names of the read tools the agent is allowed to call.';

COMMENT ON COLUMN platform_agents.actions IS
    'The names of the actions the agent is allowed to propose.';

COMMENT ON COLUMN platform_agents.enabled IS
    'Whether the agent runs; false by default, and an agent can only be enabled once it has an owner.';

COMMENT ON COLUMN platform_agents.owner_id IS
    'The operator accountable for the agent; required while it is enabled.';

COMMENT ON COLUMN platform_agents.registered_at IS
    'When the agent was first registered in the database.';

COMMENT ON COLUMN test_case_snapshots.workspace_id IS
    'The workspace that owns the test case.';

COMMENT ON COLUMN test_case_snapshots.test_case_id IS
    'The test case that was frozen.';

COMMENT ON COLUMN test_case_snapshots.user_prompt IS
    'The prompt the test case contained when the snapshot was taken.';

COMMENT ON COLUMN test_case_snapshots.acceptance_criteria IS
    'The acceptance criteria the test case contained when the snapshot was taken.';

COMMENT ON COLUMN test_case_snapshots.dataset_refs IS
    'The datasets the test case referred to when frozen, one entry per dataset with its id, file name, content hash and size.';

COMMENT ON COLUMN test_case_snapshots.content_hash IS
    'The SHA-256 of the frozen content: prompt, acceptance criteria, dataset refs, rubric and simulated user facts; identical content yields the same hash.';

COMMENT ON COLUMN test_case_snapshots.rubric IS
    'The rubric the test case had at that moment, with a version and a list of items; null when it had none.';

COMMENT ON COLUMN test_cases.workspace_id IS
    'The workspace that owns the test case; every read is scoped by it.';

COMMENT ON COLUMN test_cases.skill_id IS
    'The skill this test case checks.';

COMMENT ON COLUMN test_cases.name IS
    'The name the user gave the test case; never blank.';

COMMENT ON COLUMN test_cases.acceptance_criteria IS
    'The list of conditions a run''s result must satisfy to count as met; empty when none were written.';

COMMENT ON COLUMN test_cases.deleted_at IS
    'When the user deleted the test case; null while it is live. A deleted test case is hidden but runs that used it keep their snapshot.';

COMMENT ON COLUMN test_cases.rubric IS
    'Optional structured grading guide with a version and a list of items, each with an id, text and whether evidence is required; null when the test case is judged by its criteria alone.';

COMMENT ON COLUMN evaluation_suggestions.workspace_id IS
    'The workspace the evaluation belongs to, so every read of a suggestion is scoped to its workspace.';

COMMENT ON COLUMN test_cases.user_prompt IS
    'The request the simulated user puts to the skill at the start of a run; never blank.';

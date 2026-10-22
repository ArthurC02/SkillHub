COMMENT ON COLUMN analytics_events.event_id IS
    'Random identifier of this event; together with occurred_at it is the primary key of the partitioned table.';

COMMENT ON COLUMN analytics_events.occurred_at IS
    'When the event happened; also the partition key, so a row lives in the monthly partition containing this instant.';

COMMENT ON COLUMN analytics_events.session_id IS
    'Opaque anonymous visitor-session identifier carried in a cookie, used to group one visitor''s events into a funnel; it is not a user or login session id.';

COMMENT ON COLUMN analytics_events.workspace_id IS
    'Workspace the event was emitted for, when the visitor was signed in and the event concerns one; NULL for anonymous events, and set to NULL if the workspace is deleted.';

COMMENT ON COLUMN analytics_events.query_length IS
    'Number of characters in the search text, recorded instead of the text itself; NULL for events that are not searches.';

COMMENT ON COLUMN analytics_events.query_language IS
    'Coarse script bucket of the search text (han, latin or mixed), not a locale; NULL for events that are not searches.';

COMMENT ON COLUMN analytics_events.has_results IS
    'Whether the search returned at least one result; NULL for events that are not searches.';

COMMENT ON COLUMN analytics_events.filters_applied IS
    'Whether the visitor narrowed the search with at least one filter; NULL for events that are not searches.';

COMMENT ON COLUMN analytics_events.artifact_id IS
    'Download artifact the visitor started downloading; set only on download_started events and NULL otherwise.';

COMMENT ON COLUMN analytics_events.target IS
    'Install target chosen for a download, recorded only when the caller supplies one; the current emitter does not, so it is NULL on every existing row.';

COMMENT ON COLUMN bundle_members.bundle_version_id IS
    'Bundle version this member belongs to; members are deleted together with it.';

COMMENT ON COLUMN bundle_members.skill_id IS
    'Skill that this member represents; a bundle version holds at most one version of any skill.';

COMMENT ON COLUMN bundle_members.skill_version_id IS
    'The exact skill version pinned in this bundle version.';

COMMENT ON COLUMN bundle_members.version_number IS
    'Version number of the pinned skill version, copied so the bundle can be displayed without a join.';

COMMENT ON COLUMN bundle_members.manifest_name IS
    'Name declared in the skill''s manifest at pin time; unique within the bundle version, because it is the name the installed plugin exposes.';

COMMENT ON COLUMN bundle_members.content_hash IS
    'Content hash of the pinned skill version at pin time, copied into the bundle so the bundle''s own hash can be recomputed from its members.';

COMMENT ON COLUMN bundle_members.position IS
    'Order of this member within the bundle version; unique within it and fixes the order in which the bundle lists and hashes its members.';

COMMENT ON COLUMN bundle_versions.bundle_id IS
    'Bundle this version belongs to; versions are deleted together with the bundle.';

COMMENT ON COLUMN bundle_versions.version IS
    'Version label chosen when the bundle was released, unique within its bundle.';

COMMENT ON COLUMN bundle_versions.description IS
    'Release description the creator wrote for this bundle version.';

COMMENT ON COLUMN bundle_versions.content_hash IS
    'Hash over the ordered members'' content hashes, identifying exactly what this bundle version contains.';

COMMENT ON COLUMN bundle_versions.created_by IS
    'User who released this bundle version.';

COMMENT ON COLUMN bundles.workspace_id IS
    'Workspace that owns the bundle.';

COMMENT ON COLUMN bundles.name IS
    'Bundle name, unique within its workspace.';

COMMENT ON COLUMN cost_events.kind IS
    'Which feature made the paid call: creation_step (one step of an interactive creation session), search_embedding (embedding a search query), index_enrich (enriching a skill''s catalog metadata), review (an AI review), suggestion (an improvement suggestion), generate (generating a skill from a description), run (a trial run), match_reasons, suggest_criteria, search_intent (search-time model calls) or platform_agent (a platform agent step).';

COMMENT ON COLUMN cost_events.model IS
    'Identifier of the model that served the call, as the gateway reported it.';

COMMENT ON COLUMN cost_events.prompt_version IS
    'Revision label of the prompt used for the call; NULL when the call had no versioned prompt.';

COMMENT ON COLUMN cost_events.prompt_tokens IS
    'Number of input tokens the call consumed.';

COMMENT ON COLUMN cost_events.completion_tokens IS
    'Number of output tokens the call produced.';

COMMENT ON COLUMN cost_events.usd_micros IS
    'Provider cost of the call in micro-dollars (one millionth of a dollar), before any markup.';

COMMENT ON COLUMN cost_events.cost_source IS
    'Where usd_micros came from: gateway means the gateway reported the actual spend, estimated means the platform computed it from token counts.';

COMMENT ON COLUMN cost_events.workspace_id IS
    'Workspace the call was made for; NULL for calls not tied to one.';

COMMENT ON COLUMN cost_events.user_id IS
    'User who is charged for the call; NULL for platform-initiated calls with no payer.';

COMMENT ON COLUMN cost_events.ref_type IS
    'Kind of object the call belongs to (creation_session, run, skill_version or platform_agent_run); NULL when it belongs to none. Read together with ref_id.';

COMMENT ON COLUMN cost_events.ref_id IS
    'Identifier of the object named by ref_type; no foreign key, so it may point at something since deleted.';

COMMENT ON COLUMN cost_events.idempotency_key IS
    'Caller-supplied key that makes recording the same call twice a no-op; unique across all events.';

COMMENT ON COLUMN cost_session_summaries.session_id IS
    'Creation session this roll-up describes; one row per session.';

COMMENT ON COLUMN cost_session_summaries.user_id IS
    'User who paid for the session''s steps; NULL when none was recorded.';

COMMENT ON COLUMN cost_session_summaries.usd_micros IS
    'Total provider cost of all the session''s paid steps, in micro-dollars.';

COMMENT ON COLUMN cost_session_summaries.steps IS
    'Number of paid steps the session has made; at least one, since sessions with no paid step have no row.';

COMMENT ON COLUMN cost_session_summaries.estimated IS
    'True when at least one step''s cost was estimated rather than reported by the gateway; estimated sessions are left out of the percentile statistics.';

COMMENT ON COLUMN cost_session_summaries.last_step_at IS
    'When the session''s most recent paid step happened; decides which statistics window the session falls into.';

COMMENT ON COLUMN cost_statistics.kind IS
    'Kind of model call or session these percentiles describe; the call kinds match cost_events.kind, plus creation_session for whole creation sessions.';

COMMENT ON COLUMN cost_statistics.window_start IS
    'Inclusive start of the time window the samples were taken from.';

COMMENT ON COLUMN cost_statistics.window_end IS
    'Exclusive end of the time window; later windows supersede earlier ones, and the newest window per kind is the one in force.';

COMMENT ON COLUMN cost_statistics.sample_count IS
    'Number of measured (not estimated) samples in the window; a window with fewer than 20 samples is ignored when deciding whether a call may start.';

COMMENT ON COLUMN cost_statistics.p50_usd_micros IS
    'Median cost per sample in the window, in micro-dollars.';

COMMENT ON COLUMN cost_statistics.p90_usd_micros IS
    '90th percentile cost per sample in the window, in micro-dollars.';

COMMENT ON COLUMN cost_statistics.p95_usd_micros IS
    '95th percentile cost per sample, in micro-dollars; when the window has at least 20 measured samples and ended within the last 48 hours, it (with markup) sets the minimum balance required to start a new paid call, otherwise a configured fallback is used.';

COMMENT ON COLUMN cost_statistics.max_usd_micros IS
    'Highest cost of any sample in the window, in micro-dollars.';

COMMENT ON COLUMN creation_receipts.session_id IS
    'Creation session that received the command or attempt; the receipt is deleted with the session.';

COMMENT ON COLUMN creation_receipts.workspace_id IS
    'Workspace that owns the session; always matches the session''s own workspace.';

COMMENT ON COLUMN creation_receipts.kind IS
    'command is a request made by the person (for example confirming or finalizing); attempt is one run of background work the session started.';

COMMENT ON COLUMN creation_receipts.status IS
    'queued (accepted, not started), running, unknown (the attempt was interrupted while running, so its model cost is not known; a late result can still settle it), finished, failed or cancelled. Command receipts are written directly as finished.';

COMMENT ON COLUMN creation_receipts.expected_revision IS
    'For a command, the session revision the caller said it was acting on (a stale one is rejected); for an attempt, the session revision when the work was queued, and the worker only starts it if that still matches.';

COMMENT ON COLUMN creation_receipts.request_hash IS
    'SHA-256 (hex) of the command or attempt arguments; a retry that reuses the receipt id with a different hash is rejected as a reused command rather than replayed.';

COMMENT ON COLUMN creation_receipts.result IS
    'For a command, the view returned to the caller, stored so a retry gets the same response; for an attempt always an empty object.';

COMMENT ON COLUMN creation_receipts.usage IS
    'Model usage and cost the receipt incurred, as JSON; an empty object until it settles, and JSON null when the attempt ended without a known cost.';

COMMENT ON COLUMN creation_receipts.finished_at IS
    'When the receipt left the queued or running statuses, whether finished, failed, cancelled or unknown; NULL while queued or running.';

COMMENT ON COLUMN creation_session_events.session_id IS
    'Creation session this revision belongs to.';

COMMENT ON COLUMN creation_session_events.workspace_id IS
    'Workspace that owns the session.';

COMMENT ON COLUMN creation_session_events.revision IS
    'Session revision number this row records; strictly increasing within a session and unique per session.';

COMMENT ON COLUMN creation_session_events.event_type IS
    'Name of the transition that produced this revision: created, started, attempt_started, attempt_settled, attempt_refused, attempt_interrupted, or the kind of the command applied (such as message, confirm_brief, select_references, diagram, attach_run, raise_budget, materialize, finalize or cancel).';

COMMENT ON COLUMN creation_session_events.state IS
    'The session''s state (for example working, waiting_input, draft_ready, saved) after this revision was applied.';

COMMENT ON COLUMN creation_sessions.workspace_id IS
    'Workspace that owns the session.';

COMMENT ON COLUMN creation_sessions.state IS
    'Current stage of the session: queued, working, waiting_input, waiting_confirmation, draft_ready, candidate_ready, saved, cancelled, failed or needs_reupload.';

COMMENT ON COLUMN creation_sessions.revision IS
    'Counter that starts at 1 and increases with every accepted change; callers must present the revision they last saw, so concurrent edits cannot overwrite each other.';

COMMENT ON COLUMN creation_sessions.snapshot IS
    'The session as JSON: an envelope holding the conversation, brief, acceptance criteria, references, pending action, spend counters, current draft and candidate under "snapshot", plus the limits, deadlines and active receipt id that govern the session. The session is resumed from it.';

COMMENT ON COLUMN creation_sessions.expires_at IS
    'Fixed at creation (creation time plus the configured retention) and never extended by activity; once passed the session is treated as not found and is deleted together with its history.';

COMMENT ON COLUMN credit_accounts.user_id IS
    'User who owns the account; there is exactly one account per user.';

COMMENT ON COLUMN credit_accounts.balance_credits IS
    'Current balance in credits, equal to the sum of the user''s credit entries; may go negative, but a new paid call starts only while balance minus reservations is at or above the configured debt floor, and the database never allows a balance below -1000000.';

COMMENT ON COLUMN credit_entries.user_id IS
    'Account holder whose balance this entry changes.';

COMMENT ON COLUMN credit_entries.kind IS
    'debit is spending on model use; grant and topup are operator-posted credit increases (grant for free credits, topup for bought credits); adjustment is an operator correction and the only non-debit kind that may be negative.';

COMMENT ON COLUMN credit_entries.delta_credits IS
    'Signed change to the balance in credits: negative for debits, positive for grants and top-ups, either sign for adjustments, never zero.';

COMMENT ON COLUMN credit_entries.usd_micros IS
    'Provider cost in micro-dollars that a debit was billed for; NULL for entries that are not tied to model cost.';

COMMENT ON COLUMN credit_entries.markup_bps IS
    'Markup applied to the provider cost, in basis points where 10000 means billed at cost; NULL for entries that are not debits.';

COMMENT ON COLUMN credit_entries.model IS
    'Model that served the call this debit pays for; not filled by the current ledger writers, so NULL on every row, and the model is found on the matching cost event.';

COMMENT ON COLUMN credit_entries.prompt_version IS
    'Prompt revision of the call this debit pays for; not filled by the current ledger writers, so NULL on every row.';

COMMENT ON COLUMN credit_entries.ref_type IS
    'Kind of object the entry relates to (creation_session, run or skill_version; operator_grant is allowed but not written by current grants); NULL when none. Read together with ref_id.';

COMMENT ON COLUMN credit_entries.ref_id IS
    'Identifier of the object named by ref_type, for example the session a debit was incurred in.';

COMMENT ON COLUMN credit_entries.cost_event_id IS
    'The cost event a debit pays for; required on debits and NULL on other kinds.';

COMMENT ON COLUMN credit_entries.estimated IS
    'True when the debit was based on an estimated rather than gateway-reported cost.';

COMMENT ON COLUMN credit_entries.idempotency_key IS
    'Caller-supplied key that makes posting the same entry twice a no-op; unique across all entries.';

COMMENT ON COLUMN download_artifact_members.artifact_id IS
    'Plugin package this member is part of.';

COMMENT ON COLUMN download_artifact_members.workspace_id IS
    'Workspace that owns the package; always matches the package''s own workspace.';

COMMENT ON COLUMN download_artifact_members.skill_version_id IS
    'The skill version bundled into the package; one row per skill version in a package.';

COMMENT ON COLUMN download_artifact_members.position IS
    'Order of this skill within the plugin package, which fixes the order they are listed in the plugin.';

COMMENT ON COLUMN download_artifacts.artifact_id IS
    'Identifier of the stored artifact (the downloadable object) these package details describe; one row per artifact.';

COMMENT ON COLUMN download_artifacts.kind IS
    'Always download_package; it ties the row to artifacts of that kind.';

COMMENT ON COLUMN download_artifacts.workspace_id IS
    'Workspace that owns the artifact.';

COMMENT ON COLUMN download_artifacts.skill_version_id IS
    'Skill version a single-skill package was built from; NULL for a plugin package, whose skills are in download_artifact_members.';

COMMENT ON COLUMN download_artifacts.target IS
    'Identifier of the packaging profile the package was built for: standard, claude-code or claude-agent-sdk; multi-skill plugin packages always use standard.';

COMMENT ON COLUMN download_artifacts.profile_version IS
    'Version of the target''s packaging profile used to build a single-skill package; for a plugin package it holds the revision of the plugin specification the package follows.';

COMMENT ON COLUMN download_artifacts.packager_version IS
    'Version of the packaging code that produced the bytes; part of the key used to reuse an earlier identical package.';

COMMENT ON COLUMN download_artifacts.manifest_hash IS
    'Hash of the package''s manifest of files, identifying exactly what the package contains.';

COMMENT ON COLUMN download_artifacts.includes_test_cases IS
    'Whether the package also carries the skill''s test cases; packages that differ only here are separate artifacts.';

COMMENT ON COLUMN download_artifacts.plugin_name IS
    'Name of the plugin when the package is a multi-skill plugin; NULL for a single-skill package. Set exactly when skill_version_id is NULL.';

COMMENT ON COLUMN download_artifacts.plugin_version IS
    'Version label of the plugin when the package is a multi-skill plugin; NULL for a single-skill package.';

COMMENT ON COLUMN download_object_cleanup_intents.workspace_id IS
    'Workspace on whose behalf the object was uploaded.';

COMMENT ON COLUMN download_object_cleanup_intents.object_key IS
    'Storage key of the uploaded package object that may need deleting; unique, so a key is tracked once.';

COMMENT ON COLUMN download_object_cleanup_intents.not_before IS
    'Earliest time the cleanup may delete the object; the default leaves one hour for the package to be recorded first.';

COMMENT ON COLUMN download_object_cleanup_intents.attempted_at IS
    'When cleanup last tried to process this row; NULL if never tried. Rows never tried are picked first, so one stuck row cannot starve the others.';

COMMENT ON COLUMN download_records.workspace_id IS
    'Workspace whose member performed the download.';

COMMENT ON COLUMN download_records.artifact_id IS
    'Package that was downloaded.';

COMMENT ON COLUMN download_records.actor_user_id IS
    'User who downloaded the package.';

COMMENT ON COLUMN download_records.downloaded_at IS
    'When the download took place.';

COMMENT ON COLUMN exposure_reviews.publication_id IS
    'Publication whose public exposure was judged.';

COMMENT ON COLUMN exposure_reviews.sequence IS
    'Position of this decision in the publication''s review history, starting at 1; the highest sequence is the decision currently in force.';

COMMENT ON COLUMN exposure_reviews.release_id IS
    'The publication release the decision applies to.';

COMMENT ON COLUMN exposure_reviews.content_hash IS
    'Content hash of the release at review time, recording exactly which bytes were judged.';

COMMENT ON COLUMN exposure_reviews.snapshot_digest IS
    'Digest of the listing text a reader sees (version, name, summary, enriched summary, task examples, tags, limitations) at review time; if the current listing no longer produces the same digest, the approval stops covering it.';

COMMENT ON COLUMN exposure_reviews.decision IS
    'approved allows the release to be shown publicly; revoked withdraws that permission.';

COMMENT ON COLUMN exposure_reviews.reason IS
    'Operator''s written justification for the decision; never empty.';

COMMENT ON COLUMN exposure_reviews.reviewer_user_id IS
    'Operator who made the decision.';

COMMENT ON COLUMN exposure_reviews.reviewed_at IS
    'When the decision was made.';

COMMENT ON COLUMN feedback_reports.workspace_id IS
    'Workspace the reporter was working in; NULL if not known or the workspace was deleted.';

COMMENT ON COLUMN feedback_reports.user_id IS
    'User who sent the report; NULL if the account was removed.';

COMMENT ON COLUMN feedback_reports.kind IS
    'blocking_issue means the user could not continue; need_signal means the user is asking for something the product lacks.';

COMMENT ON COLUMN feedback_reports.message IS
    'Free text the user wrote, between 1 and 2000 characters.';

COMMENT ON COLUMN feedback_reports.page_path IS
    'Path of the page the user was on when sending, at most 512 characters; NULL if not given.';

COMMENT ON COLUMN feedback_reports.run_id IS
    'Run the report is about, when sent from a run''s page; NULL otherwise.';

COMMENT ON COLUMN feedback_reports.build_id IS
    'Identifier of the web build the user was running, to tie a report to a release; at most 64 characters, NULL if not sent.';

COMMENT ON COLUMN generation_leases.workspace_id IS
    'Workspace that currently holds the generation lease; at most one lease per workspace.';

COMMENT ON COLUMN generation_leases.expires_at IS
    'When the lease lapses on its own if the holder never released it; after this instant another generation may start.';

COMMENT ON COLUMN object_collection_queue.object_key IS
    'Storage key of a package object waiting to be checked for deletion; one entry per key.';

COMMENT ON COLUMN object_collection_queue.enqueued_at IS
    'When the key was queued, which sets the order in which the collector processes keys.';

COMMENT ON COLUMN publication_releases.publication_id IS
    'Publication this release belongs to; releases are deleted with it.';

COMMENT ON COLUMN publication_releases.skill_version_id IS
    'Skill version that was released; NULL when the release is a bundle version instead. Exactly one of this and bundle_version_id is set.';

COMMENT ON COLUMN publication_releases.version_number IS
    'Version number of the released skill version, copied at release time; NULL for bundle releases.';

COMMENT ON COLUMN publication_releases.content_hash IS
    'Content hash of what was released, fixed at release time so later changes cannot alter what the release means.';

COMMENT ON COLUMN publication_releases.findings IS
    'Result of the safety and license scan that ran before release, as JSON.';

COMMENT ON COLUMN publication_releases.rights_attested IS
    'Whether the releaser confirmed they have the right to redistribute this content.';

COMMENT ON COLUMN publication_releases.released_by IS
    'User who released it.';

COMMENT ON COLUMN publication_releases.released_at IS
    'When the release was made; the newest release of a publication is its current one.';

COMMENT ON COLUMN publication_releases.bundle_version_id IS
    'Bundle version that was released; NULL when the release is a single skill version.';

COMMENT ON COLUMN publication_releases.declared_license IS
    'License the releaser declared for this release, one of a fixed list of SPDX identifiers; NULL when none was declared.';

COMMENT ON COLUMN publications.publisher_id IS
    'Publisher under which the publication appears publicly.';

COMMENT ON COLUMN publications.name IS
    'Public name, unique among the publisher''s live publications.';

COMMENT ON COLUMN publications.skill_id IS
    'Skill being published; NULL when a bundle is published instead. Exactly one of skill_id and bundle_id is set.';

COMMENT ON COLUMN publications.status IS
    'published means listed publicly; delisted means withdrawn from the public catalog.';

COMMENT ON COLUMN publications.status_changed_at IS
    'When status last changed, or when the publication was created.';

COMMENT ON COLUMN publications.bundle_id IS
    'Bundle being published; NULL when a single skill is published.';

COMMENT ON COLUMN publications.retired_at IS
    'When the skill behind the publication was deleted, taken from the delete event time; NULL while live. The publication name stays held for 90 days after this and can then be reused. Withdrawing a publication from the catalog is status delisted, not retirement.';

COMMENT ON COLUMN publishers.workspace_id IS
    'Workspace this public identity belongs to; each workspace has at most one.';

COMMENT ON COLUMN publishers.name IS
    'Public publisher name shown on its publications, unique across the platform.';

COMMENT ON COLUMN sessions.user_id IS
    'User who is signed in with this session; sessions are deleted with the user.';

COMMENT ON COLUMN sessions.token_hash IS
    'SHA-256 digest of the random session token held in the browser cookie, so the raw token is never stored; unique.';

COMMENT ON COLUMN sessions.expires_at IS
    'When the session stops being accepted, regardless of activity.';

COMMENT ON COLUMN skill_runtime_compatibility.skill_version_id IS
    'Skill version that was measured; rows are deleted with it.';

COMMENT ON COLUMN skill_runtime_compatibility.runtime_image IS
    'Identifier of the runtime image the skill was tried on; one row per skill version and image.';

COMMENT ON COLUMN skill_runtime_compatibility.capability IS
    'Whether the run trace shows the skill being activated in that image: activated (a skill_activation event for the skill), not_activated (the run completed and the skill was offered but something else was used), or unverified (no run, or the run ended before the question could be answered; never inferred from silence).';

COMMENT ON COLUMN skill_runtime_compatibility.runtime IS
    'Whether the image provides the runtimes the package declares for its scripts: native (all provided, scripts can run as written), transpiled (not provided, so the observed result came from the model re-implementing the script rather than running it), failed, or unverified when not measured.';

COMMENT ON COLUMN skill_runtime_compatibility.source_run_id IS
    'The run whose result produced this measurement; NULL if it did not come from a run.';

COMMENT ON COLUMN skill_runtime_compatibility.measured_at IS
    'When the measurement was taken; the newest per skill version is the current answer, regardless of which image it was measured on.';

COMMENT ON COLUMN skill_sources.workspace_id IS
    'Workspace that imported or generated the package.';

COMMENT ON COLUMN skill_sources.source_type IS
    'git (fetched from a repository URL), upload (supplied directly rather than fetched: a zip archive, a saved edit or an applied improvement; no URL), or generated (written by the platform from a task description, including interactive creation).';

COMMENT ON COLUMN skill_sources.source_url IS
    'Repository address for git sources, which must have one; NULL for uploads and generated packages.';

COMMENT ON COLUMN skill_sources.fetched_at IS
    'When the package contents were obtained, which is when content_hash was taken.';

COMMENT ON COLUMN skill_sources.last_checked_at IS
    'When the origin was last re-checked for availability and changes; NULL if never checked.';

COMMENT ON COLUMN skill_sources.unavailable_since IS
    'First time a re-check found the origin gone or unreachable; NULL while it is reachable.';

COMMENT ON COLUMN skill_sources.counts_toward_generate_quota IS
    'Whether this generated package counts against the workspace''s generation allowance; false for packages from interactive creation and for sources that are not generated.';

COMMENT ON COLUMN skill_sources.plugin_name IS
    'Name of the plugin the package was found in when it was imported from a plugin; NULL otherwise.';

COMMENT ON COLUMN skill_sources.plugin_version IS
    'Version of the plugin the package was found in; NULL otherwise.';

COMMENT ON COLUMN skill_sources.plugin_repository IS
    'Repository address the plugin''s own metadata declares for itself; NULL when none was declared.';

COMMENT ON COLUMN skill_versions.workspace_id IS
    'Workspace that owns the version.';

COMMENT ON COLUMN skill_versions.skill_id IS
    'Skill this is a version of.';

COMMENT ON COLUMN skill_versions.source_id IS
    'Where this version''s package came from; NULL when it has no recorded source.';

COMMENT ON COLUMN skill_versions.version_number IS
    'Sequence number of this version within its skill, starting at 1.';

COMMENT ON COLUMN skill_versions.content_hash IS
    'Hash of the package contents; a skill never holds two versions with the same hash.';

COMMENT ON COLUMN skill_versions.source_path IS
    'Folder of the skill inside its source when the source holds several skills, relative with no leading slash; empty when the skill is at the root.';

COMMENT ON COLUMN skills.workspace_id IS
    'Workspace that owns the skill.';

COMMENT ON COLUMN skills.name IS
    'Skill name, unique within its workspace.';

COMMENT ON COLUMN skills.summary IS
    'Short description of what the skill does, shown in lists; NULL when none has been written.';

COMMENT ON COLUMN skills.forked_from_skill_id IS
    'Skill this one was forked from; NULL for skills that are not forks.';

COMMENT ON COLUMN skills.forked_from_version_id IS
    'Version of the original skill that the fork started from; NULL for skills that are not forks.';

COMMENT ON COLUMN skills.deleted_at IS
    'When the skill was deleted, by its owner or when its owner''s account is being closed; NULL for live skills. Deleted skills are hidden immediately and physically purged after a configured grace period; nothing restores them.';

COMMENT ON COLUMN skills.takedown_at IS
    'When the skill was pulled from public view by its owner or an operator; NULL while it is available. Always has a takedown_reason; lifting the takedown clears it.';

COMMENT ON COLUMN skills.takedown_reason IS
    'Why the skill was taken down; set whenever takedown_at is set.';

COMMENT ON COLUMN skills.takedown_by IS
    'Who took the skill down: owner or operator. NULL while it is not taken down. An operator takedown cannot be lifted by the owner.';

COMMENT ON COLUMN user_identities.user_id IS
    'User who owns this external login identity.';

COMMENT ON COLUMN user_identities.provider IS
    'Name of the sign-in provider, such as github, or dev for the development login; a user has at most one identity per provider.';

COMMENT ON COLUMN user_identities.provider_user_id IS
    'The user''s identifier at that provider; with provider it is unique across the platform.';

COMMENT ON COLUMN users.email IS
    'Email address of the account; unique case-insensitively among accounts that have not been deleted.';

COMMENT ON COLUMN users.display_name IS
    'Name shown for the user in the product.';

COMMENT ON COLUMN users.deleted_at IS
    'When the account closure finished: personal data was erased and the user row kept with anonymized email and display name; NULL for open accounts.';

COMMENT ON COLUMN users.deletion_requested_at IS
    'When the user asked to delete the account; NULL if no request is pending. The purge runs after this and sets deleted_at when finished.';

COMMENT ON COLUMN users.purge_attempted_at IS
    'When the purge job last tried to process this account; used to take accounts in turn so one failing account cannot block others.';

COMMENT ON COLUMN users.purge_started_at IS
    'When the purge of this account first began; NULL until it has.';

COMMENT ON COLUMN users.erasure_expedited_at IS
    'When an operator moved the purge ahead of its normal waiting period; NULL if not expedited. Can only be set while a deletion request exists.';

COMMENT ON COLUMN workspaces.owner_user_id IS
    'User who owns the workspace; each user owns exactly one.';

COMMENT ON COLUMN workspaces.name IS
    'Display name of the workspace.';

COMMENT ON COLUMN workspaces.is_catalog IS
    'True for a workspace whose skills make up the public catalog; any number of workspaces can be flagged, it is set directly in the database by operators, and ordinary user workspaces are false.';

COMMENT ON COLUMN analytics_events.event_name IS
    'Which funnel step happened: search_performed (an intent was submitted), skill_detail_viewed (a detail page was opened), session_started (a visitor came back after first use) or download_started (a download began; its success is recorded in download_records).';

COMMENT ON COLUMN analytics_events.skill_id IS
    'The platform''s own public identifier of the skill the event concerns; NULL for events about no particular skill.';

COMMENT ON COLUMN skill_sources.content_hash IS
    'Hash of the fetched package as a whole, used to tell whether a later fetch of the same source changed anything.';

COMMENT ON COLUMN skill_sources.source_ref IS
    'The commit SHA or tag the package was fetched at; NULL for uploads, which have no source revision.';

COMMENT ON COLUMN skill_versions.license_expression IS
    'The SPDX license expression the package declares; NULL when the package declares none, which the platform treats as unknown rather than permissive.';

COMMENT ON COLUMN skill_versions.manifest IS
    'The SKILL.md frontmatter as parsed at import, as JSON; an empty object when the package carried none.';

COMMENT ON COLUMN skill_versions.package_object_key IS
    'The key under which object storage holds the package itself; the row only describes it.';

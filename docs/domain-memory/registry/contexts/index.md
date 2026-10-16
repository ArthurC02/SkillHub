# Bounded Contexts

- [Workspace Activity](/contexts/activity) — Owns the Workspace-scoped cross-source activity query: contracted source coverage, completeness, global ordering, cursor semantics and typed continuation compo….
- [Product Analytics](/contexts/analytics) — Owns product-journey measurement: the funnel events and beta feedback reports that let the product learn without becoming a second source of domain truth.
- [Evaluation Batches](/contexts/batch) — Runs one Skill Version against chosen Test Cases a set number of times, answers their questions as each Test Case's simulated user, and reports pass rates with….
- [Catalog & Discovery](/contexts/catalog) — Owns Skill discovery: the search-facing projection of a Skill, hybrid lexical and vector ranking, and the tier, trust, licence and redistribution facets shown….
- [Skill Creation](/contexts/creation) — Owns interactive Skill-creation sessions: their steps, checkpoints, and the proposals produced along the way.
- [Credit Ledger](/contexts/credit) — Owns the platform's unit of account: a user's credit balance, the ledgers that explain it, and the cost statistics that decide whether a paid call may start.
- [Evaluation & Improvement](/contexts/eval) — Judges a completed trial Run against its acceptance criteria, lets a creator compare Runs and review improvement suggestions, and applies accepted suggestions….
- [Identity & Workspace](/contexts/identity) — Owns creator accounts and workspaces: login, sessions, and resolving the authenticated user and their personal workspace.
- [Trust & Supply Chain](/contexts/ingest) — Owns admitting an outside Skill package into the platform: validating its structure statically, storing the accepted archive, and recording where it came from.
- [Operations](/contexts/operations) — Owns the platform's own agents: who each one is, whether it may run, the brake that stops them all, and the record of every run.
- [Packaging & Distribution](/contexts/packaging) — Turns an immutable Skill Version into a downloadable package: it builds the archive, writes the manifest, and owns the download artifacts and records produced….
- [Policy & Usage](/contexts/policy) — Owns what a Workspace may consume and how long what it produces is kept: run and generation allowances, and download artifact retention.
- [Publishing](/contexts/publishing) — Owns who discloses what under which name: a creator's publisher namespace, the publications and Bundles in it, their immutable releases, and the operator revie….
- [Skill Registry & Versioning](/contexts/registry) — Owns Skills and their immutable versions: identity, lineage, fork, delete, takedown, and the licensing and redistribution gates on a Skill's materials.
- [Run Orchestration](/contexts/run) — Owns a Skill trial Run's lifecycle: its state machine, dispatch to a sandbox provider, following execution to completion, and cleanup.
- [Test Lab](/contexts/testlab) — Owns Test Case drafts — their prompts, acceptance criteria, rubrics and uploaded datasets — and freezes a draft into the immutable snapshot a Run executes agai….
- [Run Trace](/contexts/trace) — Owns Run trace evidence: masking, storing and reading what a Skill trial actually did.

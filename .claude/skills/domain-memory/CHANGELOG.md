# Changelog

Notable changes to the Domain Memory plugin, newest first. The format follows Keep a Changelog, and versions follow Semantic Versioning: a minor version adds or changes behavior, a patch version only fixes it. Versions before 0.10.0 are recorded in the repository history only.

## 5.1.4

### Changed

- `cargo test --test expected_output -- --bless` writes the executable's output into `engine/parity/expected/`: every case on Linux and macOS, and on Windows only the cases whose output differs from the posix recording. Text shaped like a credential, and bytes that are not UTF-8, are stored as hex.
- Three recorded cases no longer depend on the file system or the tools of the machine: no two file names that differ only by case, a home directory under `/dev/null` instead of `/proc`, and `test -e` instead of `ls` for a missing file.
- The recorded cases run git with a 300-second deadline unless the case sets its own, so a slow machine does not turn a history read into a timeout.

## 5.1.3

### Fixed

- `init-change-package --recitation` stops at a git deadline and writes nothing, as the script did; it treated the deadline as "no repository" and wrote the package.
- `scan-secrets` without git reads a repository's files in name order, so its findings come out in the same order on every file system.

## 5.1.2

### Fixed

- A git command or a test command that leaves a process holding its output open is stopped at the deadline. The executable stopped counting once the command itself exited and then waited, without limit, for the pipe to close. On Linux and macOS a git command now runs in its own process group, and the deadline kills that whole group, as the script did; on Windows the call returns at the deadline, and `taskkill /T` stops what is still attached to the command.

## 5.1.1

### Changed

- The engine is tested against the script's output recorded in `engine/parity/expected/`, by a Rust test that needs no Python: every case's stdout, stderr, exit code and written files, with a pinned clone of the repository for the cases that read its own Registry or history. CI runs it against the committed executable on Linux, macOS and Windows.
- `record-test-result` on Windows reads a test command's output as UTF-8 with replacement characters on every machine, where the script decoded it with the system code page.

## 5.1.0

### Changed

- The pre-push and pre-commit hooks run the committed executable for the platform instead of looking for a Python interpreter, and fall back to `engine/target/release/domain-memory`. When neither exists, the hook refuses and names the path and the `cargo build` command; a hook installed by an earlier version keeps calling the script until `install-git-hitl-hook` or `install-pre-commit-hook` runs again.
- AGENTS.md, SKILL.md and the command reference run every command through the executable.

### Removed

- The `domain-memory-cli` skill. How to run the commands is in AGENTS.md under "Running the commands".

## 5.0.13

### Added

- The Rust executable answers `submit-proposal`, `record-approval`, `supersede-proposal`, `verify-proposal`, `record-test-result`, `recite`, `relocate-evidence`, `harvest-candidates`, `harvest` and `discard-harvest`, with the same output, exit codes and written files as the script. It now answers every command of the script.

## 5.0.12

### Added

- The Rust executable answers `mcp-serve`: the same JSON-RPC replies, tool listing, tool results and parse-error messages as the script, one reply per line.

## 5.0.11

### Added

- The Rust executable answers `init-change-package`, `validate-change-package`, `redraft-proposal`, `rebase-proposal` and `ship-package`, with the same output, exit codes and written files as the script. `init-change-package --recitation` writes the evidence check as a call to the executable instead of the script.

## 5.0.10

### Added

- The Rust executable answers `init-domain-memory`, `confirm-sources`, `refresh-sources`, `refresh-listing`, `refine-sources`, `amend-policy`, `validate-contract`, `reconcile-audit`, `init-signing-key`, `install-git-hitl-hook` and `install-pre-commit-hook`, with the same output, exit codes and written files as the script.

## 5.0.9

### Added

- The Rust executable answers `quality-gates`, `boundary-impact`, `counterfactual` and `brief`, with the same output and exit codes as the script.

### Fixed

- The executable starts a git command's `DOMAIN_MEMORY_GIT_TIMEOUT` as soon as git starts, as the script does; on a loaded machine it could accept the result of a git call that the script reports as timed out.

## 5.0.8

### Fixed

- The executable gives up on a git command whose `DOMAIN_MEMORY_GIT_TIMEOUT` has passed even when git has already exited, as the script does; on a fast machine it could accept the result of a git call that the script reports as timed out.

## 5.0.7

### Fixed

- The plugin no longer carries text that a secret scanner reports: the engine's parity fixtures that hold credential-shaped strings are stored as hex, so importing the plugin as a Skill package no longer fails on `engine/parity/cases.json`.

## 5.0.6

### Fixed

- The executable now matches the script where its own copies disagreed: `cite` treats a combining mark (such as U+093E) as part of a word, and an invalid `DOMAIN_MEMORY_GIT_TIMEOUT` or an unreadable file stops `cite` as it stops the script; `readiness` and `probe` stop on a git deadline instead of ignoring it, and read a float or boolean limit as the script does; `verify-sources`, `probe` and `readiness` read source group paths given as text or an object as the script does.

## 5.0.5

### Added

- The Rust executable writes the Registry: `upsert-candidate`, `retract-candidate`, `promote-candidate`, `demote-local-reviews` and `recover-registry-update`, with the same output, exit codes, written files, audit events and recovery from an interrupted update as the script. Two failures where the script stops with a Python traceback (a `registry_status` that is a list or an object, and a malformed `registry_updates` in a proposal) print one `ERROR:` line instead, with the same exit code.

## 5.0.4

### Changed

- The engine's parity harness runs a command that writes files in a fresh copy of its fixture for each side and compares every file written; the script's `verify-audit` checks any audit log the executable writes. No change to either command line.

## 5.0.3

### Added

- The Rust executable answers `change-coupling`, `compare-registry`, `probe` and `readiness`, with the same output and exit codes as the script.

## 5.0.2

### Added

- The Rust executable answers `validate` and `refresh-indexes`, with the same output, exit codes and written index files as the script.

## 5.0.1

### Added

- The Rust executable answers `scan-secrets`, `discover-sources` (with `--output`), `verify-sources`, `verify-audit`, `audit-attestations`, `verify-scm-attestation`, `verify-git-governance` and `governance-readiness`. Two differences from the script remain: on Windows the executable does not descend into a directory junction while the script does, and on malformed input where the script stops with a Python traceback the executable prints one `ERROR:` line; the exit code and standard output are the same.

## 5.0.0

### Removed

- The plugin no longer reads database schemas. `describe-table`, `extract-sql-schema` and the MCP tool `describe_table` are gone, with the PostgreSQL migration reader behind them. A host that calls `describe_table` gets an unknown tool; the MCP server now offers thirteen read commands. A repository that needs its tables described or its migrations turned into candidate Rules keeps that in its own tooling.
- The `schema_conventions` section of `domain-memory-policy.json`, and its place in the policy template. A policy must drop the section: it configured only the removed migration reader.

### Changed

- `validate-policy` and `validate` refuse a top-level key the policy does not define, as `policy.<key> is not a known key`, with `; did you mean <key>?` when one is close. A leftover `schema_conventions` is refused this way, so a policy that still holds it fails both until it is dropped.

## 4.2.19

### Added

- The Rust executable answers `cite`, with `--lines`, `--function` in all seven languages, `--registry-root` and `--show`.

## 4.2.18

### Added

- The Rust executable answers `verify-evidence`, with `--path`, `--strict`, `--staged` and `--package-root`.

### Fixed

- `validate` named a concept whose `registry_status` is a list with a traceback; it now reports the invalid status.
- `validate` named a Context whose `subdomain` is a list with a traceback; it now reports the unknown subdomain.
- `verify-evidence` stopped with a traceback on a citation whose `excerpt_sha256` is a list; that citation is now stale.
- `list-proposals` stopped with a traceback when a package's `proposal_id` is a list; packages sharing such an id are now flagged as duplicates.

## 4.2.17

### Added

- The Rust executable is also committed for macOS, as one universal file for Apple silicon and Intel: `engine/bin/macos-universal/domain-memory`. The `domain-memory-cli` Skill lists it.

### Changed

- The Windows executable is linked without a timestamp, so rebuilding unchanged source gives the same bytes. `engine/build_binaries.py` builds nothing when the committed binaries already match the source.

## 4.2.16

### Added

- The Rust executable is committed for Linux on x86-64 (static, `engine/bin/linux-x86_64/domain-memory`) and Windows on x86-64 (`engine/bin/windows-x86_64/domain-memory.exe`), so a copy of the plugin runs it without a Rust toolchain. `engine/bin/manifest.json` records each one's SHA-256 and the digest of the source that built it, and `engine/build_binaries.py --check` refuses a binary that no longer matches. Other platforms still build it with `cargo`. The `domain-memory-cli` Skill now starts from the committed executable.

## 4.2.15

### Added

- `engine/`: the plugin's commands rewritten in Rust as one executable, `domain-memory`, which takes over from `scripts/registry_tools.py` one command at a time with the same arguments and byte-for-byte the same output and refusals. It answers `list-proposals`, `export`, `get-record`, `get-context`, `validate-policy`, `lookup`, `resolve-terms`, `coverage`, `analyze-boundary` and `dependents`, needs no Python, and answers each of them faster than the script; every other command refuses with `ERROR: <command> is not available in the Rust engine yet` and stays with the script. The source ships with the plugin; the executable is built locally with `cargo` and never committed.
- The `domain-memory-cli` Skill: how to build or find the executable, what it reads and prints, its exit codes, the commands it answers, and when to fall back to the Python script. Nothing changes for an Agent that keeps calling the script.

## 4.2.14

### Fixed

- `coverage` and `validate` stopped with a Python traceback when a confirmed absence named its asset as something other than text (`asset: [rules]`). `validate` now reports it as an absence of an unknown asset, as it does for any other asset it does not know, and `coverage` leaves the Context among the gaps.
- `coverage` stopped with a Python traceback when `source-map.json` listed a source group without a `kind` or without `paths` and gave no `source_kinds`. Such a group now counts as unclassified, as it already did when the map carried `source_kinds`.
- `coverage` stopped with a Python traceback when a citation written straight into the frontmatter carried `source_kind: unclassified` with a `path` that is not text. Only a path of text is listed as outside the selected sources.

## 4.2.13

### Fixed

- A field written in two places of one concept file was settled silently: a body section replaced the frontmatter value (`synonyms: 5` beside a `# Synonyms` section passed `validate`, and a frontmatter list beside the section was never searched), the frontmatter replaced the body text, and an `id` in the frontmatter replaced the file name. The concept reader now refuses the file by name and says where both copies are: `vocabulary/x.md: synonyms is written both in the frontmatter and in a body section; keep one`. The same holds for the title and a frontmatter `name`, the Relations section, `sources` and `evidence`, and the status fields. Repeating one section heading still adds to the same list.
- `validate` accepts `synonyms: null` and `not_same_as: null` as absent, as the other readers have since 4.2.12; it used to report `synonyms and not_same_as must be lists of non-empty strings`.

## 4.2.12

### Fixed

- `synonyms` that is not a list of labels stopped `resolve-terms` with a Python traceback when it was a number or a boolean, and when it was one word each letter counted as a synonym, so `resolve-terms --query order` reported a term with `synonyms: orders` as a synonym match. The concept reader now refuses it by file name and exits 1: `vocabulary/x.md: synonyms must be a list of labels, got a string`. `synonyms: null` reads as absent.

## 4.2.11

### Fixed

- A relation field of the wrong shape stopped `validate` and `get-context` with a Python traceback on 64 of 140 shapes tried (a number, a mapping, a nested list, a list where one id belongs). The concept reader now refuses it by file name and exits 1: `rules/x.md: contexts must be a list of Context ids, got a number`, `interactions/x.md: producer_context must be one Context id, got a list`, `rules/x.md: contexts must be a list of Context ids, got a list holding an object`. A list relation written as one word used to match by substring; it is refused the same way.
- A relation set to `null` reads as absent: `get-context` and `resolve-terms --context` no longer crash on `contexts: null` or `consumer_contexts: null`, and `validate` reports the missing field instead.
- `validate-policy` reports a `format` that is a list or a mapping as an invalid format, `policy has an invalid format: [] is not one of domain-memory-policy/v1`, instead of crashing.

## 4.2.10

### Fixed

- `get-record` and every reader of the audit log name the file that is not UTF-8, as the Registry loader has since 4.2.5: `contexts/orders.md: 'utf-8' codec ...` and `audit/events.jsonl: 'utf-8' codec ...`. Both used to report only the codec error, so the reader could not tell which file to fix; `verify-audit` reports the same named reason.

## 4.2.9

### Fixed

- A bare `-` in a frontmatter sequence is accepted only when a deeper block follows it as its value. Before, a bare `-` in the middle of a sequence took the next `- item` as its own nested list (`[["b"]]` where YAML reads `[null, "b"]`), and one before a shallower key swallowed that key into the sequence, both without a message; each is now refused like a bare `-` at the end: `rules/x.md: frontmatter line N: a '-' entry has no value`.

## 4.2.8

### Fixed

- Three concept-file shapes stopped every Registry read with a Python traceback; each is now refused by file name and exits 1 like any other unreadable concept. A frontmatter sequence whose last row is a bare `-` reads `rules/x.md: frontmatter line 4: a '-' entry has no value`; `sources` that is not a list (a number, a word, a mapping) reads `rules/x.md: sources must be a list of cited sources`, where a word or a mapping used to turn silently into one evidence entry per character or key; and a `status` that is not text reads `rules/x.md: status must be text`.

## 4.2.7

### Changed

- `brief --base` builds the dependency graph, the default statuses and the readable Change Packages once per brief instead of once per changed record; lists the files changed since the base once instead of twice; hands the records it already holds to each Context model instead of reading them again per Context; and a Context's bound rules compare against each collaboration's cited paths computed once. A brief over 227 changed records took 15 s here and takes 2.5 s; output is unchanged. `boundary_impact`, `touched_records`, `context_model`, `proof_of` and `dependents` keep their signatures and gain optional arguments for records a caller already holds.

## 4.2.6

### Changed

- A command imports only the modules it runs: the dispatcher loads the one command module that holds the handler, the parser no longer imports the review, harvest and counterfactual machinery for three constants, and the SQL reader, the MCP server, the search and the review modules load inside the handlers that use them. Behaviour and output are unchanged; the saving is import time on every invocation.

## 4.2.5

### Changed

- Within one command the concept files are read from disk once: the conflict-marker scan and the Registry loader share the same bytes instead of each opening every file. Output is unchanged.

### Fixed

- A concept file that is not UTF-8 is refused by name (`contexts/orders.md: 'utf-8' codec ...`); `validate` used to report the decode failure without the file name, raised from the index check after the load had already failed.

## 4.2.4

### Changed

- `validate`, `readiness` and `coverage` read each Registry document once per command, as `brief` has since 4.2.2; `readiness` used to decode the Registry three times (for its validation, its coverage and its record count). Output is unchanged.

## 4.2.3

### Changed

- `brief` refuses a wrong subject in words that fit what was given: with neither `--base` nor a record it says what it needs; with both it says not both; with `--asset` or `--id` alone it asks for the other. One sentence used to cover all three and read oddly when nothing had been given.

## 4.2.2

### Changed

- `brief --base` reads the base revision's Registry as one `git archive` instead of one `cat-file` object per concept file, and reads each current Registry document once per brief instead of once per record it mentions. On a Registry of 1,400 concept files the brief goes from 9.3 s to under 3 s; its output is unchanged.
- When git has already told the repository walk which entries it ignores, discovery and quality gates no longer ask `git check-ignore` about the files that walk returned; they still ask when the walk could not consult git, so the refusal when ignored files cannot be told from sources is kept for that case.

## 4.2.1

### Changed

- The evidence verifier resolves each cited file once instead of three times: its callers already hand it the resolved repository root and the resolved path, so the escape check compares those directly. On a repository whose disk answers slowly this was a third of `verify-evidence` and a quarter of `validate`; verdicts are unchanged.

## 4.2.0

### Changed

- The repository walk no longer enters directories or lists files that git ignores. Every reader already treated an ignored file as not a source, not a gate and not a counted test, but found that out after walking it; the walk now asks git once (`git ls-files --others --ignored --exclude-standard --directory`) and prunes on the way down, so a repository whose ignored caches hold 88,000 of its 108,000 files is walked in under a second instead of six. Untracked files that git does not ignore are still listed, and a directory that is not a git repository is still walked whole.

## 4.1.2

### Changed

- One repository walk per command: `validate` walked the repository once for its errors and once for its NOTE, every Registry write validated the staged and the current Registry with a walk each, and `readiness` walked once for discovery and once for `validate`; each now lists the repository once and hands the listing on. A Context's `implementation_path` is also normalised once instead of once per directory. `validate` on a repository of 108,000 files and 18 Contexts goes from 17 s to 7 s, `readiness` from 14 s to 8 s, and a write saves two walks.
- `describe-table --base` reads the base revision's migrations from the git archive in memory instead of extracting them to a temporary directory and reading them back; `coverage` classifies evidence with one source-kind index instead of one per record; `quality-gates` names hook files without building a path object per repository file; `verify-evidence` hashes a file's windows once per excerpt length instead of once per moved citation. Each prints what it printed before.

## 4.1.1

### Changed

- The reader behind `describe-table --queries` is split into `sql_statements_under`, which reads a directory of `.sql` files once, and `statements_naming`, which matches one table against what was read, so a tool that measures every table reads the query files once instead of once per table; `describe-table` prints the same `queries`.

## 4.1.0

### Added

- `describe-table` without `--table` or `--base` prints the catalog: every live table with where it is declared, its meaning and the counts of its columns, undescribed columns and gaps, so an agent finds the table before reading one whole description; `--require-complete` exits 1 while any table has a gap.
- `describe-table --queries <dir>` lists, under `queries`, each statement in the `.sql` files under that directory that reads or writes the table, with its line, its verb and the table's columns it uses, so the agent sees how the table is used today; a NOTE says when none does.
- `history` on a described table: the migrations that created, altered, indexed, triggered or commented on it, in application order.
- `mcp-serve` offers `describe_table`, with `migrations` and `queries` read relative to the repository.

### Changed

- The SQL reader scans statements, table bodies and clauses by the characters that matter instead of one character at a time; the 144 migrations of a 73-table schema read in 0.56 s instead of 0.90 s with an identical result.
- `validate` and `readiness` matched every Context's `implementation_path` against every repository file by building a path object for each pair; the distinct directories are collected once and matched as text, with the same answers. The repository walk behind discovery, `validate`, `quality-gates` and `verify-sources` no longer stats every directory a second time. On a repository of 108,000 files and 18 Contexts, `validate` took 106 s and takes 17 s; `readiness` took 65 s and takes 14 s.

## 4.0.2

### Fixed

- `cite --function` read a line that starts with `name(` as a definition in TypeScript and JavaScript, so a function called at the start of a line was reported defined several times; the parameter list is now read to its closing bracket and only `{`, a `: type` followed by `{`, or `=>` after it makes a definition. Measured on a web application's 109 top-level arrow functions against the TypeScript compiler, 14 refusals of this kind are gone.
- An arrow function's expression body ended at the first balanced brace pair, so a template literal `${}` or an object literal inside it cut the range short; it ends at the semicolon that closes the statement, with strings, template literals, comments and brackets read as units. The same reader ends a block body at its matching brace, which a brace inside a string no longer disturbs.
- An arrow function whose parameter list spans lines, such as a destructured object type, was refused; the arrow is looked for after the list closes, wherever that is.
- A regular-expression literal is read as a unit, so a `//` or `/*` inside it is not a comment; a Rust lifetime (`'a`) is not read as the start of a string; a Go function returning a tuple, a Java method with `throws`, and a Kotlin expression body are read to their ends.

## 4.0.1

### Fixed

- `cite --function` finds a TypeScript or JavaScript arrow function whose parameters start on the line after `const name =`, including a curried one with an expression body; a `const` whose next line holds no arrow is not taken for one.
- The refusal for a name the file does not define says it looked for the forms `--function` reads and points a definition of another form to `--lines`, instead of reading as if the function did not exist.

## 4.0.0

### Removed

- The `github-pr` verifier and the `external-scm` trigger, with the provider API calls behind them, `--verification-token-env` on `ship-package` and `audit-attestations`, the `GITHUB_TOKEN` default and the `pull_request` and `checks_url` attestation fields. The plugin verifies git signatures only, offline, in any clone on any host or none; a policy that still names `github-pr` is refused by `validate` and `validate-policy` with the `amend-policy` that moves it to `git-signed-commit`. `discover-sources` recommends `git-signed-commit` with `git-push` for every repository and still lists the CI it finds.

### Added

- `review_governance.distinct_signer` (false unless set): the attested commit's signer must be someone other than its author, read from git; set it with `init-domain-memory --distinct-signer` or `amend-policy --set distinct_signer=true`. The pre-push hook does not apply it.
- `project_profile` in the policy: `implementation_roots`, `excluded_directories` (added to the built-in list), `decision_roots` and `boundary_markers`, given to `discover-sources` and `init-domain-memory` as the repeatable `--implementation-root`, `--excluded-directory`, `--decision-root` and `--boundary-marker`, amended with `amend-policy`, and read by discovery, `readiness`, the source snapshots and `refresh-listing`. Absent, the defaults are the previous behaviour, so an existing policy validates unchanged; a bad section is refused one line per defect.
- `cite --function` reads TypeScript, JavaScript, Java, Kotlin and Rust beside Go and Python, ending a brace body at its closing brace; a file of another kind is refused with the list and `--lines`.

### Changed

- `ci_requirement` states what the project's own pipeline runs (`verify-git-governance`, `audit-attestations`); the plugin no longer reads a provider's check runs.

## 3.0.0

### Removed

Every input form that existed beside the one the reference names. Each is refused by argparse now, so a script that still uses one stops at once with `unrecognized arguments`.

- `cite --start <line> --end <line>`: write `--lines <start>-<end>`.
- `amend-policy --field <field> --value <value>`: write `--set <field>=<value>`.
- `record-test-result --command`, which only refused a command that differed from the obligation's `check`; the check is what runs.
- A harvest spec's `enforcing` span as `[path, start, end]`: write `path:start-end`.
- `--repo-root` on the ten Registry readers that never used it (`lookup`, `resolve-terms`, `get-context`, `get-record`, `dependents`, `analyze-boundary`, `verify-audit`, `reconcile-audit`, `refresh-indexes`, `init-change-package`); a host that passed it to every command drops it for these.

## 2.10.1

### Changed

- One `canonical_json` and one `sha256_of` in `common.py` replace the three copies the audit log, the Registry revision and citations kept; the bytes they produce are unchanged, so every recorded digest still verifies (checked against a 665-event audit log and its Registry digest before and after).
- One `checked_git` / `checked_git_bytes` pair in `git.py` runs a git command and refuses with the caller's sentence on a failure, a timeout or a non-zero exit, replacing five hand-written checks and the attestation module's own runner; each refusal reads as before.
- One `inside` in `common.py` answers whether a path resolves under a root, replacing eleven inline checks; each caller keeps its own refusal.

## 2.10.0

### Changed

- The agent-facing pages say each thing once. The seven code rules live on `AGENTS.md` alone and the hygiene Skill points there; rule 7 names the gaps and leaves their definitions to `describe-table`'s reference, which is now a structured section instead of one paragraph. `AGENTS.md` keeps a one-line-per-task command table and no longer restates the write route, the MCP registration (now under `mcp-serve`) or the review path in full.
- `references/host-integration.md` keeps what a host must do and drops the capability sequence and event table that repeated the Skills. `references/packaging.md` is gone; its maintainer facts (manifest fields, evals, release check) are in `README.md`. `references/reliability-architecture.md` points to the pages that own record and proposal statuses and the approval authority instead of restating them. The seven-step workflow's harvest section points to `harvest` and `counterfactual` instead of restating them, and readiness-first is stated on `AGENTS.md`, the router and the Read and Extract Skills only.
- `references/readiness.md` maps each `next_capability` value to the Skill or commands to open; two of the values open no Skill, which no page said before.
- The Read, Maintain and Review Skills link the tactical questions instead of paraphrasing them; the Skill descriptions no longer collide (review: before relying on a fact; review-change: when asked to review a PR or branch; extract: the Registry has never seen the source; maintain: merged code moved cited lines; hygiene: while writing code).
- `references/script-api.md` opens with an index of its 64 commands, and the notes on reading a counterfactual result sit under `counterfactual`; the protocol-revision details under `mcp-serve` are removed.

## 2.9.2

### Fixed

- The term tokenizer read only ASCII letters, so an ideographic word vanished and `café` became `caf`; it now normalizes to NFKC, keeps every Unicode letter and digit, searches an ideographic run character by character (a multi-character term is found wherever all its characters occur) and keeps an accented word whole, so `lookup`, `resolve-terms` and `--with-mentions` work on prose in any script.
- Three places still read a boolean as a line number or an audit sequence (`compare`, `boundary-impact` and `reconcile-audit`); they use the same whole-number rule as the rest.
- `init-signing-key` and `audit-attestations` print non-ASCII text as written instead of `\uXXXX` escapes.
- The packaging page counted six code rules where there are seven; the host-integration page said no leaf Skill restates readiness while Read and Extract do; the Maintain Skill credited `probe` with naming moved sources, which `verify-sources` does under `changed_sources`.
- The requirements now say that `init-signing-key` alone needs OpenSSH's `ssh-keygen`, and its reference names the Git config keys it sets.

## 2.9.1

### Fixed

- A bad `schema_conventions` section is refused one line per defect, naming the key, the index and the value: an unknown key suggests the closest known one, a value that is not a list says what it is, a blank or non-string entry names its position, and the limit's rule reads "a whole number 0 or more" with the value given; every defect is reported in one run instead of the first.

## 2.9.0

### Added

- `domain-memory-policy.json` takes an optional `schema_conventions` section (`exempt_columns`, `status_like_suffixes`, `free_text_markers`, `value_comment_limit`), so a project names the columns that need no comment, the suffixes that make a column status-like, the phrases that mark a column as free text and the list length up to which every value must be explained; `describe-table` applies it when `--registry-root` is given and the defaults otherwise. `validate` names each defect in the section. New Registries get the section with its defaults written out.

### Changed

- The API reference states the migration shape the reader understands (PostgreSQL DDL, one directory of forward-only files in name order) and the shapes it does not, and the reference pages use neutral example names.

## 2.8.5

### Fixed

- A `--migrations` directory that does not exist, or holds no `.sql` file, was reported as a missing table ("no table 'runs' in .../db/nope"); `describe-table` and `extract-sql-schema` now refuse the directory by name before looking anything up.

## 2.8.4

### Fixed

- A relative `--migrations` path given to `describe-table` or `extract-sql-schema` resolved against the working directory, so a run from another repository read that repository's migrations and died on a raw path error; it now resolves against `--repo-root`, which the citations are relative to anyway, and a directory outside the repo root is refused with a sentence that says how to pass it.

## 2.8.3

### Fixed

- The run-time DDL gap can now close: a later migration that declares the trigger the template names with a static `CREATE TRIGGER` on a table supersedes the dynamic statement for that table, so `describe-table` lists no `dynamic_ddl`, raises no gap and prints no NOTE for it, and `--base` reports the gap as closed. A trigger declared before the dynamic statement, a trigger of another name, or a template whose trigger name is itself a placeholder does not supersede it. The gap sentence now says what clears it.

## 2.8.2

### Fixed

- A nullable column whose value list is a bare `IN (...)` is no longer asked to decide NULL when another CHECK on the table already does, such as `CHECK ((cost_usd IS NULL) = (cost_source IS NULL))` or `CHECK (takedown_by IS NULL OR takedown_at IS NOT NULL)`; the decision was written, only in a different constraint.
- A column declared with two inline CHECKs, such as `qty int CHECK (qty > 0) CHECK (qty < 9)`, gave both constraints the whole declaration as text, so neither body parsed: the value list, the NULL decision and the allowed values went missing. Each check now also carries its own `clause`, which `describe-table` prints and the readers parse.

## 2.8.1

### Fixed

- The free-text escape for a status-like column now requires the comment to begin with `free text` or `free-text` (any case), so a comment such as `Not free text: one of the vendor phases` no longer escapes the value-list gap and the common hyphenated spelling does.
- `describe-table --table <name> --base <ref>` scopes the report and the exit code to that table instead of silently ignoring `--table`; a table the migrations do not create is refused.

## 2.8.0

### Added

- `describe-table --base <ref>` lists, per table, the gaps the migrations since that Git ref opened and closed, and `--require-complete` then exits 1 only for a new gap, so a change to a table with an older backlog is still gated on what it changed.
- The status-like column names now include `phase`, `stage`, `level`, `priority`, `severity`, `visibility` and `category`; a column of such a name that is genuinely open escapes the value-list gap when its comment says `free text` or another CHECK constrains it.

### Fixed

- A listed value counts as explained only when the comment mentions it as a whole word (`running` no longer explains `run`, `pro_plus` no longer explains `pro`).
- A nullable column of an enum type is no longer asked to decide NULL in a CHECK: the type cannot hold an unlisted value and the nullability is already explicit.

## 2.7.0

### Added

- `describe-table` lists `gaps`, what keeps a table from being complete: a table or column without a `COMMENT ON` (`id`, `created_at` and `updated_at` need none), a status-like column without a value list, a value list on a nullable column whose CHECK decides nothing about NULL (a bare `IN (...)` lets NULL through), a listed value the column's or type's comment does not explain, a foreign key without `ON DELETE`, and run-time DDL naming the table. `--require-complete` exits 1 while gaps remain, so an agent adding or altering a table has a check to pass rather than prose to remember; the implementation-hygiene skill and AGENTS.md name it as the seventh condition of a source change.
- A Registry record whose evidence cites a constraint, key, unique index or trigger that a later migration dropped or replaced is marked `cites_replaced` in `describe-table`, with the constraint and the migration that retired it, and a NOTE names such records; the reader keeps every retired declaration with the statement that retired it.
- A `DO` block or function body that executes DDL built at run time is recorded: `describe-table` lists it under `dynamic_ddl` for each table it names and counts it as a gap, and `extract-sql-schema` lists it under `dynamic_ddl` with a NOTE, so a description is never silently incomplete.

## 2.6.0

### Added

- The SQL reader names every constraint the way the database names an unnamed one, so `describe-table` and a `DROP CONSTRAINT` agree with the catalog: `<table>_pkey`, `<table>_<columns>_key`, `<table>_<columns>_fkey` with every key column, `<table>_<column>_check` when the CHECK mentions one column and `<table>_check` otherwise, `<table>_<columns>_excl`, a number appended to the label on a collision (`_check1`, `_check2`), and the two names shortened the way the database shortens them to fit 63 bytes. A column with two inline constraints keeps both.
- `DROP CONSTRAINT` by any of those names removes the constraint, including an inline `REFERENCES` and an unnamed table-level `UNIQUE`; `DROP COLUMN` removes every constraint, foreign key and index that involves the column, as the database does; `DROP TABLE` removes the table's constraints and keys.
- `describe-table` lists the primary key and each foreign key among `constraints` with its name; `references` and `referenced_by` carry the key's `name`, `columns`, `parent_columns` (the parent's primary key when the migration names none), `on_delete` and `on_update` (`NO ACTION` when unwritten); each column carries its `default` (following `SET DEFAULT` and `DROP DEFAULT`) and `at`, the file and line that declared it, which for a column added by `ALTER TABLE` is that statement; `indexes` lists the table's `CREATE INDEX` statements, unique or not, with their definition.
- An unknown table is refused with the name quoted, so stray whitespace shows, and the command strips the `--table` argument first.

### Changed

- `extract-sql-schema` keeps its rule ids and statements for inline constraints (`<table>.<column>`), while a rule from an unnamed table-level constraint is now identified by the database's name for it (`<table>_check`, `<table>_<columns>_key`) instead of `<table>:<line>`; candidates recorded under the old line-numbered ids are not renamed and can be retracted after the next extraction.

## 2.5.4

### Fixed

- A CHECK of the form `<column> IS NOT NULL AND <column> IN (...)` is read as that column's value list, as `<column> IS NULL OR <column> IN (...)` already was; the null clause only says whether the column is required.

## 2.5.3

### Added

- The SQL reader records whether a column is required (`NOT NULL`, an inline or table-level `PRIMARY KEY`, and later `ALTER COLUMN ... SET NOT NULL` or `DROP NOT NULL`) and whether it is part of the primary key; `describe-table` shows them as `required` and `primary_key` on each column.

## 2.5.2

### Fixed

- A CHECK that is exactly `<column> = '<value>'`, optionally allowing NULL, is read as a value list of one, so `describe-table` reports the one allowed value instead of none.

## 2.5.1

### Fixed

- The SQL reader joins a `COMMENT ON` written as adjacent string literals (`'one ' 'two'`) into one text instead of keeping only the first literal, so `describe-table` shows the whole meaning and a column is not reported as missing values its comment does name.
- `describe-table` reported no `allowed_values` for a column typed with a `CREATE TYPE ... AS ENUM`; the SQL reader now records each column's declared `type` and each enum type's values in order, including those an `ALTER TYPE ... ADD VALUE` appends, and such a column lists them when no CHECK lists its values. The description carries the column's `type` and, when the migrations give the type a `COMMENT ON TYPE`, its `type_meaning`, which the reader records per type and which is the one place to say what each value of an enum shared by several columns means.
- A comma inside a trailing `--` comment or inside a string literal in a `CREATE TABLE` body no longer starts a phantom column, and `ADD COLUMN IF NOT EXISTS` no longer names its inline constraint after the word `IF`. A column's recorded `type` keeps all its words and parentheses (`double precision`, `numeric(12, 6)`, `text[]`).
- `DROP CONSTRAINT` by the name the database gives an inline column constraint (`<table>_<column>_check`, `<table>_<column>_key`) removes that inline declaration, so a constraint re-declared under that name is read once, not twice, and `extract-sql-schema` does not offer the dropped one as a rule.

## 2.5.0

### Added

- `describe-table --repo-root <repo> --migrations <dir> --table <name> [--registry-root <root>]` describes one table for the agent about to read or write it: the table's and each column's `COMMENT ON` meaning, the values a CHECK allows, the constraints and unique indexes in force, the tables it references and is referenced by, its partitions, and every Registry record that names it or cites its lines, with status, so a draft is told from a reviewed fact. Columns with no meaning in the files are listed and named in a NOTE. The SQL reader now keeps the text of `COMMENT ON TABLE` and `COMMENT ON COLUMN` rather than only the fact that one exists.

## 2.4.3

### Fixed

- The test that a counterfactual whose test run never finishes is inconclusive gave the intact baseline run five seconds, which a loaded machine can miss, so the baseline failed instead of the mutated run hanging; it now allows fifteen seconds against a sixty-second hang.

## 2.4.2

### Changed

- The pattern-verification reference says what to mutate when the cited source is compiled into a generated file, such as a SQL query a generator embeds in Go: the generated file the test runs, or a `--test-command` that regenerates first, since mutating the source alone leaves the counterfactual surviving for the wrong reason.
- A failing `verify-evidence` is covered by a test that reads its whole stdout as one JSON document and finds the failure line on stderr only, after a report that stdout had carried more than the JSON; the report could not be reproduced in any mode, and the test holds the contract.

### Fixed

- The test that a grandchild holding the output open is stopped at the deadline gave the hanging command two seconds to start two interpreters, which a loaded machine can miss, and then failed on a missing pid file; it now allows six seconds and names the spawn race when the pid file is still absent.

## 2.4.1

### Added

- `retract-candidate` takes `--id` more than once and removes every named candidate of the asset in one Registry update with one audit event listing their ids, so a batch of extraction candidates is withdrawn in one validation pass rather than one per record; `discard-harvest` retracts a package's drafts the same way. A missing or reviewed id refuses the whole batch.
- `promote-candidate --drop-cite PATH:START-END`, repeatable, removes the citation of those lines from the promoted record's evidence, so an amendment can drop lines the code no longer holds instead of re-merging a stale citation from the reviewed record; it refuses lines the record does not cite.

### Changed

- A Registry update validates the Registry it started from only when the updated copy fails validation, to tell new errors from inherited ones, and validation builds the expected indexes from the records it has already loaded, so an update on a large Registry reads each record once rather than four times.

### Fixed

- `extract-sql-schema` no longer writes a rule for a non-unique index, partial or not: such an index only serves queries and states no rule, and reviewers were retracting every one of them. A unique index still yields a rule.
- The SQL reader follows `ALTER TABLE ... RENAME COLUMN`, so a `COMMENT ON COLUMN` under the new name is attributed to the column instead of being dropped, and a column counts as described only by `COMMENT ON COLUMN`, which the database catalog holds, not by a `--` comment beside its declaration.

## 2.4.0

### Added

- `extract-sql-schema` reads a directory of SQL migrations the way a database applies them and writes one candidate Rule per CHECK, UNIQUE and EXCLUDE constraint, unique or partial index and trigger a table still carries, citing the migration lines that declare it; the Context comes from `--table-context TABLE=CONTEXT` or `--default-context`, a table neither names is listed in `unplaced_tables`, a reviewed rule of the same id is skipped, and statements the reader cannot parse are listed in `unread_statements` rather than silently dropped. The reader keeps what the database would hold at the end: a constraint, index or trigger declared again under its name replaces the earlier declaration, and `DROP CONSTRAINT`, `DROP INDEX` and `DROP TRIGGER` remove one, so a rule cites the declaration in force. The Registry learns the rules a schema enforces without the plugin ever connecting to a database.

## 2.3.9

### Fixed

- `promote-candidate` run again for a record, with or without `--amend`, keeps the citations an earlier run's `--cite` or `--cite-function` added to the package's copy; it rebuilt the entry from the Registry and dropped them without a word.
- A `--cite` naming lines the promoted record already cites replaces that citation in place instead of appending a second one, so a citation gone stale is refreshed by citing it again; the two were told apart by their excerpt digest, which is what changes when the lines change.
- `record-approval` takes `--registry-root` and `--repo-root`, which it does not read, as `submit-proposal`, `record-test-result`, `verify-proposal` and `ship-package` do; the flow's flags no longer fail at the approval step with `unrecognized arguments`.

## 2.3.8

### Fixed

- `readiness` without `--registry-root` no longer reports that the repository holds no Domain Memory when git could not list them: with no git on the path, or a directory that is not a repository, `domain_memories` is now `null` and a `NOTE:` gives git's reason (`NOTE: git could not list the Domain Memories this repository holds: <reason>; pass --registry-root to assess one.`); it listed `[]` and assessed nothing, as it does for a repository that truly holds none. The hint under `no Domain Memory at …` still falls back to the neighbouring directories as before.

## 2.3.7

### Fixed

- A git deadline surfaces as one `ERROR:` line from every command, as it has since 2.2.3 for discovery and the quality gates. Six readers still swallowed it as if git were absent: `submit-proposal` stamped a base revision with no commit, so `verify-proposal` and `rebase-proposal` later said the base names no commit without saying why; `confirm-sources` and `refresh-sources` recorded an empty git state, so every later `probe` and `verify-sources` took the slow hashing path; `readiness` listed no Domain Memory, `scan-secrets` walked the tree instead of asking git, the untracked prefilter of `discover-sources` was skipped, and a re-citation package named absolute paths in its check. Each now ends in the deadline's own line, which names `DOMAIN_MEMORY_GIT_TIMEOUT`; a git that cannot start at all still takes the fallback it took before.
- A re-citation package asks git for the repository top before it writes anything, so a deadline there leaves no half-written package behind.

### Added

- Tests that each of the six readers lets a git deadline through and still falls back when git cannot start.

## 2.3.6

### Fixed

- `validate` checks the `domain-memory-policy.json` beside the Registry, as `validate-policy` does, and fails on what it finds; it said `Registry is valid.` and exited 0 over a policy with `storage_mode: "cloud"`, so a CI or an agent running only `validate` passed a Domain Memory whose policy was broken. A policy that is not JSON is named the same way; a Registry without a policy file is checked as before. Like git conflict markers, a refused policy is reported alone, since the policy decides what the rest of the check requires.

## 2.3.5

### Changed

- A field with a closed set of values names the value it was given and the values it takes when it is refused, as an unknown subdomain already did: `domain-change-proposal.json has an invalid change_classification: 'refactor' is not one of material, routine`. The same holds for a proposal's status, a test obligation's status, a record's status, and a policy's format, storage_mode, data_classification and review_mode; before, each said only `has an invalid <field>` and sent the reader to the schema.

## 2.3.4

### Changed

- `discard-harvest` says what it already did: it withdraws any Change Package that is not applied, harvested or written by hand. Its help reads `Withdraw a Change Package that is not applied, written by harvest or by hand`, its refusals say `package` where they said `harvest` (`only a draft, submitted or verified package is discarded`, `discarding a package requires the reason it is undone`), and the review path in AGENTS.md names it, so a submitted package a refactor left stale is withdrawn with one command instead of by deleting the directory and restoring the audit log by hand.

### Added

- A test that a submitted package written by hand is withdrawn: its candidate retracted, the directory removed and the reason in the audit event.

## 2.3.3

### Fixed

- The test that a git deadline ends in one error line no longer depends on git taking longer than a millisecond: on a fast runner every git call finished in time and the command ended on another error. It now raises the deadline itself; the real timeout is tested with commands that do run long.

## 2.3.2

### Added

- A test that an approved proposal whose `finalized_at` is absent or not a timestamp does not apply.

## 2.3.1

### Added

- Tests pinning the next step `harvest` names for a draft and for a run stopped with its test results, and the one `ship-package` names for an approved package whose apply failed and for a package finalize sends back.

## 2.3.0

### Added

- `counterfactual --package-root <package> --obligation-id <id>` records a killed run's `counterfactual_check` in the package's `evidence-bundle.json` and prints `recorded_in`, replacing the run recorded for the same obligation and a `not_applicable` entry and keeping the others as a list. The package's status and the obligation are checked before anything runs; a run that is not killed records nothing and prints `not_recorded`. Only `harvest` filled the check before, so a package written by hand needed it copied in.

## 2.2.17

### Removed

- `hooks_dir`'s own absolute-path branch: joining an absolute path onto the repository root already yields it.

### Added

- Tests for the hooks directory Git names, relative and absolute, and for the conventional `.git/hooks` taken when Git names none, where installing a hook without that directory is refused.

## 2.2.16

### Added

- Tests for the four refusals `init-domain-memory` makes before writing anything: scm-verified without a verifier, signed commits without a signer, signed commits checked on commit instead of on push, and an output outside the repository.

## 2.2.15

### Fixed

- A JSON `true` is no longer read as the number 1: a policy limit, a `proposal_revision`, the lines of a citation, the lines of a harvest `enforcing` span and a committed audit manifest's event count given as `true` are refused like any other value that is not a whole number.

### Added

- Tests for a policy broken in every field, naming each defect in field order, and for consistency holders that are not objects or whose consistency is not text.

## 2.2.14

### Fixed

- `verify-git-governance` (and the pre-push hook) accepts a signed merge commit that joins two lines which each changed Domain Memory, such as the merge `reconcile-audit` resolves; it read the merge's changed files without naming a parent, got none, and refused it with `Git commit does not change Domain Memory files`. A merge is now compared with its first parent.

## 2.2.13

### Added

- Tests for a Taskfile and a golangci configuration that are not UTF-8, and for `cite --function` placing a Python `async def` and a Go generic func.

## 2.2.12

### Added

- Counterfactual tests: a suite entry whose `cwd`, `expect` or `obligation_id` is not text, an entry overriding the command the suite sets for every entry, and a `--cwd` naming a file.

## 2.2.11

### Added

- Tests for `brief` given an asset without an id, an id without an asset, or a base with an asset, and for `dependents` walking out of a context and skipping a reference to a record the registry lacks.

## 2.2.10

### Added

- Lookup tests: a query of stop words only finds nothing and says `none`, and the result limit holds for records holding every word of the query and for records holding some of its words.

## 2.2.9

### Added

- Boundary tests: an index description at 160 characters kept whole and at 161 shortened, failure evidence keeping 40 failure lines and dropping the 41st, and a possible rule pair offered at a similarity of 0.25 and not just under it.

## 2.2.8

### Fixed

- `harvest --spec` names a killed counterfactual that lacks its `counterfactual_check` (`the counterfactual output has no counterfactual_check; give the whole output of counterfactual`); it said `the counterfactual is killed, not killed`.

### Added

- Tests for discarding a harvest by a spec without a name or with a blank reason, superseding a verified or approved proposal and naming a blank replacement, an approval time given in ISO-8601, an environment signing key against an existing key file with and without `--force` and signing every commit, refining to no source, refreshing a confirmed selection, and a source map without a valid selection.

## 2.2.7

### Fixed

- `audit-attestations` refuses a `--changes-root` that does not exist (`no Change Packages directory at <path>; pass the directory that holds them`) instead of reporting every package carried and exiting 0; an existing empty directory still finds nothing.
- `verify-evidence` without `--staged` counts one failing citation in the singular (`1 stale citation`), as `--staged` does.

### Removed

- The base-revision check for a Registry file that changed outside its records: the concept layout fixes a document's status and the manifest, so the check could never fire.

### Added

- Tests for a Registry update that fails at each step or succeeds leaving nothing behind, an audit log at and past its size limit, the secret scan's exit code at and past the file size limit, the staged summary in the plural and for an unhashed citation, and the rerun advice of a git deadline.

## 2.2.6

### Fixed

- A registry recovery journal that is not JSON is refused as `registry recovery journal is not JSON`, instead of leaking the JSON decoder's message without the file's name.

### Added

- Tests for the refusals of a source selection outside the repository, missing or empty; each way a record file cannot be read; a recovery journal that is not JSON, too large or not an object; the exact reasons a base revision is stale or cannot be compared; a record brief with nothing to point out and a Context the Registry lacks; and a configured hooks path that does not exist.

## 2.2.5

### Fixed

- `reconcile-audit` on a Domain Memory that has no audit log yet reports nothing to join and writes nothing, instead of failing on the missing `audit/` directory.

### Changed

- Fourteen tests that asserted a substring, a prefix or only that nothing was raised now assert the exact message, exit code or record; the compare note is held at exactly half, the counterfactual exit code at either mutation order, and the package-script and ESLint gates on their positive side.

## 2.2.4

### Added

- Tests for branches the suite never reached: a policy that requires no CI, a policy without a review verifier, attesting a commit the repository does not hold, a check that gives no result in time, the latest review of a reviewer deciding the approval, appending to a broken audit chain, reciting a path a record does not cite, a Context's prefixes and implementation path, a git deadline on the command line, and `--since` with a git that cannot answer.

## 2.2.3

### Fixed

- A cited line ends at a line feed only, as editors and git count lines; a form feed, a U+2028 separator or a lone carriage return no longer starts a line, so cited line numbers match the editor's.
- Discovery and the quality gates refuse with `git check-ignore failed in <root>, so ignored files cannot be told from sources: <stderr>` when git cannot say which files are ignored, instead of listing the ignored ones as sources; a git deadline surfaces as such.
- `counterfactual` refuses a suite entry whose `timeout` is `true` or `false`; a boolean passed as one second before.
- `run_tests.py` exits 1 and says `nothing ran` when no `test_*.py` is under its root, and fails a module that exits non-zero even if it printed `Ran 0 tests`.

### Added

- A pre-commit hook test for a cited path holding a space or non-ASCII letters.

## 2.2.2

### Removed

- Forty tests that could not go red under any product change or repeated a stronger test on the same branch; the suite keeps one test per condition.

## 2.2.1

### Added

- Tests for the refusals the suite never exercised: each defect `verify-proposal` finds in the test evidence, submitting a package that is not a draft or names no proposer, verifying one that is not submitted, an update that expects a Registry revision the Registry has left, and each defect in a Context's `confirmed_absences`. Removing any of those checks now turns a test red.

## 2.2.0

### Fixed

- A concept file reads a decimal or exponent number back as a number; it read `1.5` back as the text `1.5`. A number that is not finite is refused on write.
- A line under a section heading that is not a `- ` item is refused by name, instead of being dropped on read and lost on the next write.
- A `=======` line on its own, a setext heading underline, is no longer taken for a merge conflict marker; `<<<<<<< `, `>>>>>>> ` and `||||||| ` still are. The audit log, whose every line is JSON, still strips it when reconciling a conflicted merge.
- `readiness --registry-root` naming a path that does not exist reports the Registry `absent` instead of failing on the missing source map.
- Records read in the same order from the working tree and from a commit when one id prefixes another, such as `order` and `order-line`, so the two digests agree.
- `compare-registry` no longer pairs two Contexts whose `implementation_path` is blank.
- `mcp-serve` answers a `tools/call` whose `name` is not text with an invalid-params error instead of an internal error.
- Rolling back an interrupted install removes its staging directory, and a journal whose staging directory is gone asks for reconciliation instead of failing midway.
- `relocate-evidence` says that a citation without a digest, or with an empty range, cannot be found, instead of claiming its content changed.

## 2.1.10

### Changed

- The implementation hygiene page no longer restates the runtime, which the entry page holds, nor its own first three rules under *Check the expression*; that section keeps the two cases the rules do not cover.

## 2.1.9

### Removed

- The Registry authoring and Registry maintenance pages. The authoring page's asset table repeated the schema's and the extraction page's, and it still pointed at the record templates 2.0.0 removed; its guidance on choosing an Interaction over a Dependency policy is in the Registry schema, and its smallest-slice advice in the extraction page. The maintenance page's sequence repeated the Maintain capability's; what it added, that evidence says where a statement came from and not that the rule still holds, and what `probe` reports, is on the Maintain page.

## 2.1.8

### Fixed

- The reliability page said a test result entered by hand does not count; `verify-proposal` accepts one that carries the run's output digest and a link to the CI run, as the command reference states, and the page now says so.

## 2.1.7

### Changed

- The extraction page keeps how to read `change-coupling` and `compare-registry`, and the command reference holds what their flags do and how `compare-registry` pairs records; the bullets that repeated the reference's mechanics are gone.

## 2.1.6

### Fixed

- The test runner gives each test module a git timeout of 60 seconds unless the environment sets `DOMAIN_MEMORY_GIT_TIMEOUT`; the 15-second default the commands keep had `ssh-keygen` time out on a loaded Windows runner and fail a run that changed no code.

## 2.1.5

### Changed

- The reliability architecture page states what the scripts do, not a target: the write protocol and its recovery as the journal phases run them, the proposal states by what each proves, the two verifiers by what they check, and the limits as refusals. Code-owner checks, Unicode normalization of identifiers, contract structural diffs, secret scanning before every write, partial results over a limit and marking a proposal applied only after its commit were described and not implemented; they are gone.

## 2.1.4

### Fixed

- Three sentences in the command reference that pointed `brief`, `install-pre-commit-hook` and `mcp-serve` at their own notes since 2.1.0 are gone. The output conventions, the walking and timeout rules and the shape of an update have their own headings, *Output and refusals* and *The shape of an update*, instead of sitting under *Checks on the code being written*, and the entry page links the output conventions again; 2.1.2 dropped that link as misdirected when only its heading was.

## 2.1.3

### Changed

- The harvest spec format, what a harvest writes and what a resume skips sit under `harvest` in the command reference, and the counterfactual suite file format under `counterfactual`, instead of in the workflow page; the workflow page keeps when to harvest, the loop by hand, and how to sign and ship one, and links to the reference for the formats.

## 2.1.2

### Changed

- The entry page opens with the two rules that hold whoever is reading, then says what is on disk and how a fact changes, and every remaining paragraph says when an Agent reads it and what it does next; the write route is one paragraph, the four honesty checks, the closing manifesto and the misdirected link from the command form to the counterfactual section are gone, and what makes a package a Context sits with the vocabulary. The common-tasks table keeps each command and the step after it; what a flag does lives in the command reference only. The reference no longer calls the Registry JSON files.

## 2.1.1

### Fixed

- The test holding each command's `--help` line to the reference reads the reference's one-section-per-command layout; 2.1.0 shipped with it reading the old table and failing.

## 2.1.0

### Changed

- The command reference gives each command its own section, the synopsis first and then what it guarantees, instead of a table whose cells ran to a page each; the notes that sat apart for `brief`, `install-pre-commit-hook` and `mcp-serve` now follow their commands. The two groups, their order and the wording are unchanged, and the anchors other pages use still resolve.

## 2.0.2

### Changed

- The router page says, for each capability, when to use it and nothing else: the paragraphs that only advertised a reference are gone, since each capability page links the references it uses, and one closing line says where the workflow, the command reference, host integration, the reliability architecture and packaging are. Each capability's description now states when to use it. The extract page no longer mentions `init`.

## 2.0.1

### Fixed

- The AGENTS.md task table names `--registry-root` on the `init-change-package --recitation` line, the one command whose need for it the table's convention did not cover. A test now parses every invocation in that table, with the two root flags the convention leaves out, so a flag the parser does not take, or one it requires, is caught before a reader is; it caught three more lines, which now name `--package-root` for `ship-package`, `--reason` for `discard-harvest` and `--path` with `--lines` for `cite --show`.

## 2.0.0

### Removed

- The JSON Registry layout, one file per asset, which no version since 0.30.0 writes. A Registry is a `registry/index.md` with one concept file per record; `migrate-registry` is gone with it, and a Registry still kept as JSON is converted by the 1.x line before upgrading. `brief`, `export` and `compare-registry` read a commit's Registry in the concept layout only. The `templates/domain-registry` skeleton went with the layout.
- Plain `path:line` citations and `migrate-evidence`: a citation is an object built by `cite`, and anything else is `invalid`, so `verify-evidence` no longer reports `legacy-unverified`, and `compare-registry` matches structured citations only.
- The `review` block a record carried before `verified`: nothing strips or reads it any more, and `validate` refuses a record holding one.
- `init`, which copied an empty skeleton that could never hold a reviewed record: `init-domain-memory` is the entry point.
- `readiness` no longer reports a `reviewed-empty` Registry, a state only the JSON manifest could express.
- `scripts/build_standalone.py` and the standalone bundle: the Plugin is the one distribution.
- A transaction journal without `phase` and `operation_id`, written before journaled phases, is malformed instead of recovered.

### Changed

- A missing Registry is named as `it holds no registry/index.md`, and a directory is a Domain Memory when it holds `registry/index.md` or a source map; a bare `registry/manifest.json` no longer counts.
- Without a policy file `review_mode` is `no-policy`.

### Fixed

- `promote-candidate --cite` no longer adds a citation the record already holds: a citation is the same when its path, lines and excerpt digest are.

## 1.3.9

### Removed

- `search.resolve_terms`, a limited wrapper no command called: `resolve-terms` reads every match itself so it can say how many the limit cut. The MCP server's `READ_ONLY` list, which only a test read: the test pins the offered tools by name instead.

## 1.3.8

### Changed

- The shifted-citation note agrees with its count: `relocate-evidence re-cites it` for one citation, `them` for several.

## 1.3.7

### Changed

- `validate-change-package --repo-root` reports a shifted citation, whose lines are unchanged but sit elsewhere in the file, as one `NOTE: registry update <asset>/<id> cites N citations whose lines are unchanged but now sit elsewhere in <files>; ...` per record, instead of one `ERROR:` per citation whose text said it passes every check; the exit code no longer counts it.

## 1.3.6

### Fixed

- A re-citation package that records no approval, the routine package `init-change-package --recitation` writes, could not be applied: apply stamped `verified` from the proposal's approvals, found none, and the staged Registry failed with `rules/<id> is reviewed but missing verified` after the commit was signed. Apply now keeps the record's own `verified` when the proposal records no approval and the update changes nothing but citations; an approval still replaces it.

### Changed

- `verify-proposal`, `validate-change-package --as-status verified` and finalize refuse, before anything is signed, an update that once applied would be reviewed by nobody: `once applied: rules/<id> would be reviewed by nobody; the proposal records no approval and the Registry holds no verified rules/<id>, so record-approval first`, or `... and changes more than its citations (statement), so record-approval first`.
- `init-change-package` prints the created path in POSIX form.

## 1.3.5

### Changed

- `harvest` refusing to re-run a finished harvest names the way out: for a verified package, `to harvest again, discard-harvest --package-root <path> --reason <why>, then run harvest again`; for an approved or applied one, `give the spec another name to harvest it as a new package`.
- `discard-harvest` takes `--spec <file> [--changes-root <dir>]` in place of `--package-root`, and finds the package where `harvest` put it for that spec, so the pair takes the same arguments; giving neither or both is refused.

## 1.3.4

### Changed

- When `cite --function` or a harvest's `test` names a function the cited file does not define, the refusal looks the name up in the repository's files of the same language and adds where it lives, `; it is defined in internal/api/generate_test.go:360`, or `; it is defined in several files: a.go:12, b.py:3` when several define it. Before, the refusal stopped at the file that lacked it, and finding the right file was a search by hand.

## 1.3.3

### Changed

- The `reconcile-audit --registry-root` and `refresh-indexes --registry-root` hints echo the root in POSIX form, `C:/Users/.../memory`, so the command pastes back into any shell; before, on Windows the resolved path came back with backslashes, JSON-escaped to `C:\\Users\\...`.
- `verify-audit` and `reconcile-audit` refuse a root without a Registry with the same sentence every other command uses, naming both Registry markers and the nearest Domain Memory when one is found, instead of their own `point --registry-root at the directory that holds registry/manifest.json`.

## 1.3.2

### Added

- `refresh-indexes --registry-root <path>` rewrites `registry/index.md` and each `registry/<asset>/index.md` from the concept files beside them and prints the indexes it rewrote, so an index a merge conflicted in is rebuilt rather than resolved by hand. After a merge the procedure is `reconcile-audit`, `refresh-indexes`, `validate`.

### Changed

- `validate` refuses a Registry while any file under `registry/`, under `changes/` or any JSON file at the root holds git conflict markers, naming every such file with its line, `registry/contexts/orders.md holds git conflict markers at line 3; resolve the merge before validating`, and reports nothing else until they are gone. Before, a marked concept file surfaced as a decoding error that named no cause.
- `validate` refuses an asset index that does not list the concept files beside it, `registry/contexts/index.md does not list the records beside it; run refresh-indexes --registry-root <path>`.
- `verify-audit`, and every command that reads the log, report a conflict-marker line as `conflict markers at event N; run reconcile-audit --registry-root <path>` instead of `audit event N is invalid JSON`.

## 1.3.1

### Changed

- `lookup`, `resolve-terms`, `get-context`, `get-record`, `dependents`, `analyze-boundary`, `verify-audit`, `reconcile-audit` and `init-change-package` accept `--repo-root` and leave it unused, as `coverage` already did, so the flags most commands need can be passed to every command.
- An argument a command does not take is refused with that command's own usage line, `usage: ... get-context [-h] --registry-root ... --id ID [--summary]`, instead of the top-level `usage: ... command ...`, which named no flag the command accepts.

## 1.3.0

### Added

- `reconcile-audit --registry-root <path>` repairs `audit/` after a git merge brought two clones' events together: it drops git conflict-marker lines, keeps each event once, rewrites `events.jsonl` in height order and `manifest.json` from the events, and joins several heads with one `reconcile-audit` event naming them in `joined`.

### Changed

- The audit log is a hash-linked DAG, as a git history is, instead of a single chain. `previous_event_sha256` names the event an event was appended on, or a sorted list of events when it was appended on several heads, and `sequence` is the event's height, one more than its highest parent. Verification resolves parents by digest, so file order no longer matters, a repeated event is refused, and the manifest names `heads` (`head_sha256` when there is one, null while there are several). Every existing log verifies unchanged: a chain is a DAG with one parent each. Before, two clones that each appended on the same base produced two chains that could not be merged, even when the Registry content was identical.
- `verify-audit --against` requires the log to hold every head the committed manifest names and at least as many events, instead of the committed head at the committed position.

## 1.2.32

### Changed

- The hint a refusal gives for a missing Domain Memory, `check --registry-root (...)`, now states the one rule: a Registry holds `registry/index.md` or `registry/manifest.json`, or a `domain-source-map/v1` `source-map.json`. It used to say a Registry holds `registry/` and `source-map.json`, the loose rule the code no longer applies.

## 1.2.31

### Fixed

- What counts as a Domain Memory is now one rule, in the code and in the reference: a `registry/` holding `index.md` or `manifest.json`, or a `source-map.json` of format `domain-source-map/v1`. The git-backed search already required a marker file inside `registry/`, while the reference said a path "carries a `registry/`" and the neighbour search outside git accepted any `registry/` directory, so a bare directory of that name counted in one search and not the other.

## 1.2.30

### Fixed

- A `source-map.json` that is not a Domain Memory source map, such as a build artifact or a file that is not JSON, no longer counts as a Domain Memory when `readiness` looks for one or a refusal says "did you mean"; only a file whose `format` is `domain-source-map/v1`, or a `registry/` directory, does. Before, such a file sent `readiness` to the "several Domain Memories" branch and dropped the Registry signals until `--registry-root` was passed.

## 1.2.29

### Added

- `readiness` without `--registry-root` looks for the Domain Memories the repository holds: one is assessed as if named, reported in `registry_root` and announced on stderr; several are listed in `domain_memories` with a note asking for `--registry-root`. Before, the first `readiness` in an unfamiliar repository, the run the guidance asks for, said nothing about a Registry sitting at a conventional path.

## 1.2.28

### Changed

- `readiness` with a usable Registry adds a `listing` signal, `current` or `drifted` with `discovered_but_gone` and `discovered_but_unlisted`, and on drift prints the `NOTE:` that `verify-sources` prints, with the `refresh-listing` to run; the listing is never a block.
- `refresh-listing` prints an `added:` and a `removed:` line naming the paths after its count line.
- A refused `harvest` citation that discovery would place today shows the `refresh-listing` roots relative to the repository, and the report's `next` leads with that command.

### Fixed

- `discovered_but_gone` counted only listed paths whose file was deleted, so a listed path discovery no longer places, such as an instruction file name discovery stopped listing, was absent from the drift note while `refresh-listing` dropped it; the count under-stated what a refresh removes. It now names every listed path discovery no longer finds, which is what the refresh drops.

## 1.2.27

### Added

- `discover-sources` lists the database schema roots `db`, `database`, `migrations` and `schema` as implementation, so a rule a migration's constraint or trigger enforces can cite the migration once `refresh-listing` has brought the listing up to date. Before, a repository that kept its schema outside `apps`, `src`, `services` and `packages` could not cite it at all, and a harvest had to cite the decision describing the rule in place of the lines enforcing it.
- When `harvest` refuses a citation outside the confirmed sources and today's discovery would place that path in a group, the refusal names the group and the `refresh-listing` to run.

## 1.2.26

### Removed

- `discover-sources` no longer lists `CLAUDE.md` as a repository instruction file. The instruction source is `AGENTS.md` at any depth, the file name every coding agent reads; one host's own instruction file is that host's concern, and the Plugin carries nothing that belongs to one host.

## 1.2.25

### Changed

- When a `go test -v` or `-json` run passes no test, the refusal now says why: `no test passed: every test was skipped (<count>), the first because <the skip's own message>` when tests were skipped, so a test that skips itself for a missing environment variable names it, and `no test passed: no test matched` otherwise. It used to say `no test passed: every test was skipped or none matched` in both cases.

## 1.2.24

### Fixed

- `harvest` promoted an amended or reproven rule as the Registry held it and only appended the test citation, so the reviewed record's citations, stale ones included, and its old statement and examples went into the package, and the package could not verify. It now promotes the spec's statement, examples, contexts and citations, each cited afresh from the current lines, in place of what the reviewed record held.

## 1.2.23

### Added

- `verify-evidence --package-root <package>` checks the Registry as that Change Package's `registry_updates` would leave it: an upsert replaces or adds the record, a remove drops it.

### Fixed

- A re-citation package for a `stale` citation could never pass its one obligation, because the generated `verify-evidence` command checked the Registry as stored, which still held the old digests, and `ship-package` needs a passed result before it applies the package; a `shifted` citation only passed because `verify-evidence` does not fail on it. `init-change-package --recitation` now writes the check with `--package-root` pointing at the package, so the same command passes before apply and `verify-evidence` on the Registry is clean after ship.

## 1.2.22

### Fixed

- The `generic-secret` kind of `scan-secrets` fired on any assignment to a secret-sounding name, including `token = unescaper.Replace(token)`; it now takes a quoted literal or a bare value and leaves a call, a member access or an index on the right-hand side alone.
- The API reference said exit 2 means an incomplete scan; it means an incomplete scan that found nothing, since a finding exits 1 either way.

## 1.2.21

### Fixed

- `scan-secrets` walked the whole directory tree, so inside a repository it read ignored paths such as virtual environments and build caches, took minutes, hit its limits, and reported third-party code as findings. Inside a repository it now reads the files git tracks or would add (`git ls-files --cached --others --exclude-standard`) and walks the directory only outside one.
- A binary file, recognized by a NUL byte in its first 8 KiB, is read no further instead of being searched line by line.

## 1.2.20

### Changed

- The MCP server reports the version from `scripts/domain_registry/version.py` instead of reading one host's manifest directory, so a standalone bundle reports its real version and the server no longer assumes `.claude-plugin/` exists; a test keeps that file equal to the manifest.
- Discovery recommends the `github-pr` verifier only when the pipeline it reads is present (`.github/workflows`); any other CI gets `git-signed-commit` with the `git-push` trigger, since the pull-request verifier reads one hosting provider's API and the policy pairs a signed-commit verifier with push enforcement.
- The secret scanner also finds GitLab personal access tokens (`glpat-`), AWS access key ids (`AKIA`/`ASIA`) and Slack tokens (`xox?-`), reported as `gitlab-token`, `aws-access-key` and `slack-token`.

### Removed

- `.devctl` is no longer a built-in excluded directory when walking a repository; it was one project's tool directory, and a repository that ignores it in `.gitignore` is still skipped as every ignored path is.

## 1.2.19

### Added

- The pre-push and pre-commit hooks export `git config domain-memory.git-timeout` as `DOMAIN_MEMORY_GIT_TIMEOUT` when the environment does not set it, so a slow machine sets the git limit once for every hook run.

### Fixed

- The did-you-mean search matched any file whose name ends in `source-map.json`, so a `foo-source-map.json` could put a folder that holds no Domain Memory in the suggestion, and it missed a Registry that has no source map yet; it now asks git for files named exactly `source-map.json`, `registry/index.md` or `registry/manifest.json`, the same marks `holds_domain_memory` reads on disk.
- `validate` printed its no-Registry refusal with absolute paths while `probe` and `confirm-sources` printed paths relative to the repository; given `--repo-root` it now prints the same relative form (`no Domain Registry at elsewhere: ...; did you mean memory?`).

## 1.2.18

### Fixed

- The probe summary printed the given root in the platform's separator (`Domain Memory at docs\domain-memry: ...`) while its did-you-mean used forward slashes; the prefix now goes through the same formatter as the refusals, a forward-slash path relative to the repository root.
- The probe's "none" line closed its sentence with a period even when the did-you-mean's question mark already ended it (`did you mean memory?.`); the period is dropped when the line ends on the question.

## 1.2.17

### Fixed

- A refusal for a root that holds no Domain Memory looked only at the parent and the children of the root it was given, so a typo beside the Registry, or the repository root itself, got no did-you-mean. When the repository root is known the refusal now asks git for every source-map.json the repository holds, at any depth: exactly one is offered as a did-you-mean, and several are listed as "Domain Memories in this repository: a, b". Outside a repository the siblings of the given root join its parent and children in the search.
- The same refusal printed the given root in the platform's separator while the did-you-mean used forward slashes; both now print as a forward-slash path relative to the repository root when it is known (`no Domain Memory at memry: ...; did you mean memory?`), and as an absolute forward-slash path otherwise.

## 1.2.16

### Fixed

- The test of the did-you-mean refusal compared against the path as the test typed it, while the command prints the root it resolved, which differs on a Windows runner whose temporary directory is a short name; the test now compares against the resolved path. No product change.

## 1.2.15

### Fixed

- The test of the advised-path quoting built its sample path with Windows separators, which a POSIX file system keeps as literal characters, so the plugin's test suite failed on Linux from 1.2.13; the sample is now built from path segments. No product change.

## 1.2.14

### Fixed

- A refusal on a missing source map or `registry/` no longer ends with "initialize the Domain Memory before ...", which led an agent with a mistyped `--registry-root` to create a second, empty Registry at the wrong path. Every such refusal (`confirm-sources`, `refresh-sources`, `refresh-listing`, the Registry reads and `validate`, and `probe`'s "none" line) now ends "check --registry-root (a Registry holds registry/ and source-map.json); initialize one only if this repository has none yet", and names a Domain Memory found one directory up or down from the path given: "; did you mean <path>?". The sentence for a root that is the `registry/` directory itself is replaced by that did-you-mean.

## 1.2.13

### Fixed

- The discovery-drift advice of `probe` and `verify-sources` prints the real paths, as given on the command line and with forward slashes, quoted only when a value holds a space, so the line pastes as a working `refresh-listing` command; before, it printed `<root>` and `<repo>` as placeholders.

## 1.2.12

### Changed

- The discovery-drift advice of `probe` and `verify-sources` reads `run refresh-listing --registry-root <root> --repo-root <repo> (no confirmation needed)`, naming the flags the command takes, since the reader otherwise copies the flags of the command that printed it.

## 1.2.11

### Fixed

- On Windows a git timeout now stops every process the call started, through a job object the command is assigned to as it starts; the shell-based hooks, fsmonitor and credential helpers of Git for Windows escaped the parent-child walk the previous release relied on. A process spawned in the instant before the assignment still escapes; `taskkill` remains the fallback when no job can be assigned, and a daemon git leaves behind on success is not touched.
- `probe` reports discovery drift as `verify-sources` does, with the same counts and the `refresh-listing` remedy in its sentence, and keeps exit 0; before, it said "current" for a tree `verify-sources` reported as `discovery_drifted`.
- The command reference states that the git deadline is per call, so a command that runs git several times can wait several times the limit.

## 1.2.10

### Changed

- `probe` on a stale selection says that selected sources gained, lost or renamed files, which is what `verify-sources` reports, instead of "moved"; and a current selection whose files changed only inside is reported with their count and `verify-evidence` named, where before the sentence said nothing.

### Fixed

- The git deadline can be raised: `DOMAIN_MEMORY_GIT_TIMEOUT` gives the number of seconds, default 15, and a value that is not a positive number is refused before any command starts. The timeout refusal names the variable and, for a git read, says that nothing was written, so running the command again is safe.

## 1.2.9

### Changed

- Every git and ssh-keygen command runs through one module, `git.py`, instead of nine hand-made wrappers spread over the source-map, store, attestation and signing code.
- On Windows, a package outside the repository is shown as a forward-slash path in `ship-package`, `harvest`, `rebase-proposal` and the re-citation package, as `recite` already showed it.
- Searching the Registry (`lookup`, `resolve-terms`, prose mentions) lives in its own module, `search.py`, with no change in behaviour.

### Fixed

- A git command that times out returns at the deadline: it stops the whole process tree git started (a hook, credential helper or fsmonitor) instead of waiting until that process lets go of the output. A process that detaches into its own session still escapes, and the call then returns after a five-second drain.
- A Registry commit git cannot read is refused as `git could not read <commit>: <what git said>` instead of "subsection not found", and a listed file git reports missing is skipped instead of crashing the read.
- A Change Package whose proposal is not valid JSON is named in the refusal of `coverage`, `get-record`, `brief`, `verify-audit` and `audit-attestations` (`Change Package <name> cannot be read: …`); `list-proposals` still lists it as an error row.
- A stale base that cannot be compared says why: the commit does not hold the Registry, the committed Registry does not reproduce the recorded digest, or the base names no commit; each names `rebase-proposal`.
- `coverage` counts a JSON-layout record as reviewed when its document says reviewed, as `stage_candidates` already did.
- `mcp-serve` no longer crashes when its input stream cannot be re-encoded.

## 1.2.8

### Changed

- The command reference's longest rows now hold only the contract (flags, verdicts, exit codes, output keys, refusal texts). The reasoning moved to the pages that carry each topic: the harvest spec and resume rules to the seven-step workflow, the pairing and threshold reasoning to extraction, the citation advice to the evidence rules, and the notes on `brief`, the pre-commit hook and `mcp-serve` to a section at the end of the reference.
- `init-change-package` and `validate-change-package` are one row each. The `probe` row states its exit codes (0 current, 1 any other status, 2 absent). The long paragraphs in six reference pages are split into one rule per bullet.

## 1.2.7

### Fixed

- A git or ssh-keygen call that hangs now gives up after the git timeout, and a timeout ends in one `ERROR:` line instead of a Python traceback.
- `init-domain-memory` whose policy is refused no longer leaves a half-built Registry behind, so rerunning with corrected options works instead of failing with "output directory is not empty".
- A stale proposal base is recognised by its error type rather than by matching message text, so `rebase-proposal` and `ship-package` keep telling a stale base from other errors if the wording changes.
- `verify-proposal` and `finalize-proposal` validate the package before writing. An invalid package is never marked verified or approved on disk, not even briefly; a failed finalize still moves the proposal back to submitted, in one write.
- `discard-harvest` records its audit event under the writer lock before removing the package, so a busy Registry leaves the package in place instead of deleting it without a record.
- Deeply nested JSON is refused as `invalid JSON: <path>` instead of crashing; a committed policy that is not JSON is refused as `<commit>:<path> is not valid JSON`; and a GitHub review or pull request whose `user` or `head` is null is treated as no approval or a head mismatch instead of crashing.
- A signing key taken from `DOMAIN_MEMORY_SIGNING_KEY` is created readable by its owner only, so there is no moment when other users can read it.
- An occupied output directory is refused with the same `ValueError` everywhere, instead of `FileExistsError` in three places.

## 1.2.6

### Fixed

- The documents now say what the code does: dependency policies take `forbidden`, `readiness` needs `--registry-root` to report `extract` or `dead`, and the output convention names the commands that print JSON or a sentence.
- Quoted commands now carry the flags the parser requires, and a drifted reviewed record goes to a Change Package through `promote-candidate --amend` rather than to `upsert-candidate`, which refuses it.
- The capability count, the definition of a material change, the lifecycle rows and the reliability reference now match the files and the commands that exist.

### Changed

- `probe` on a stale selection now says to run `refresh-sources` and then have a developer run `confirm-sources`, instead of "refresh before trusting it".

## 1.2.5

### Changed

- The lint ratchet now applies every selected rule family to the tests as well, except the boolean-argument rules (`FBT`), which tests trip by passing booleans to the product API; the fourteen findings that the old exemptions hid are fixed (nested `with` statements, unused mock arguments, one dict comprehension). The `evals` exclusion covers only the deliberately broken fixtures, so `check.py` is linted too.
- A golden test pins the digest of an empty Registry to a literal, so a change to the canonical JSON that feeds `registry_digest` is caught before it turns every committed package's base stale.

## 1.2.4

### Fixed

- `rebase-proposal` on a proposal whose base is current prints the package with `note` `the proposal base is current; nothing was written` and `next` for its status, and exits 0, in place of `ERROR: the proposal base is current; nothing to rebase` with exit 1: a current base is the same harmless no-op as a verified package under `verify-proposal`, and a script that rebases defensively before shipping must not read it as a failure. A draft, applied or superseded proposal is still refused, since each needs another command.

## 1.2.3

### Fixed

- `rebase-proposal` judges "current" by the stale check verify, finalize and apply run, not by equality of the whole base revision. Before, a commit on top that touched nothing the proposal names, or a pre-1.2.0 base without `committed_digest`, made it rebase a current proposal and retire a valid approval.
- `ship-package` runs the stale check before attesting, so a stale base stops with nothing written instead of leaving an attestation behind, and its `next` then names `rebase-proposal` and the steps after it.

### Changed

- `rebase-proposal` also takes an `approved` proposal, since its approval and attestation bind to the base that went stale; it returns to `submitted` with `finalized_at` cleared. Only a draft, applied or superseded proposal is refused.

## 1.2.2

### Fixed

- `verify-proposal` on an approved or applied package prints `already <status> at revision <n>; nothing was written` and exits 0, like the verified case since 1.2.1: a verify after shipping is the same harmless re-run. A draft is still refused, since it needs submitting.

## 1.2.1

### Fixed

- `verify-proposal` run again on a verified package prints `already verified at revision <n>; nothing was written` and exits 0, as `ship-package` does for an applied one, instead of refusing with "only a submitted proposal may be verified"; a re-run after success is not a failure.

## 1.2.0

### Fixed

- A proposal's base revision now records, beside the digest of the Registry it was submitted against, `committed_digest`: the digest of the Registry its observed commit holds. A harvest writes its drafts and submits in one run, so the submitted-against digest always described uncommitted content and could never be rebuilt from the commit; once another package's apply or another harvest's drafts changed the Registry, the proposal was refused as "cannot be rebuilt" for good. The base is now rebuilt from the commit and compared, so several harvests can be open at once and ship in turn.
- A record the proposal itself writes no longer counts as changed when the Registry holds it as the draft the reviewer saw. When it does differ, the refusal reads `<asset>/<id>, which this proposal writes, changed since it was reviewed`. Records the proposal removes or only names must still be as they were.
- The stale-base refusal names its remedy: `proposal base revision is stale; rebase-proposal, then obtain fresh approval: <what changed>`. The apply step's own race refusal says `run ship-package again`, which is what it needs.

### Added

- `rebase-proposal --package-root <path> --registry-root <root> --repo-root <repo>` restamps a submitted or verified proposal on the current Registry: it raises `proposal_revision`, so earlier approvals stay as history and stop counting, removes the attestation that named the old revision, returns the proposal to `submitted` with its test results kept, and prints what to do next. It writes only the package and refuses a draft, approved, applied or superseded proposal, and one whose base is current.

### Changed

- `discard-harvest` also undoes a verified harvest that will not be shipped; approved and applied packages are still refused.
- A package submitted before this version carries no `committed_digest`; it keeps the old check, and when the Registry changed after it was submitted from uncommitted drafts, `rebase-proposal` is the way forward.

## 1.1.1

### Fixed

- `run_tests.py` marks a module in which no test ran as `skip` instead of `ok` or `FAIL`, so the helper module `test_support` no longer fails the run on Python 3.12 and later, where unittest exits 5 when it ran nothing. The line stays visible, so a module whose tests vanished is still noticed.
- Two hints read as instructions an agent would follow literally. The pre-commit refusal now ends with `details: verify-evidence … --staged … prints the full report, as this hook does when DOMAIN_MEMORY_HOOK_VERBOSE is set` instead of telling the user to commit again with the variable set. The `verify-sources` drift NOTE for a map whose selection is not current says why it points at `refresh-sources`: `the selection is <status>, so run refresh-sources, then confirm-sources`; a `discovery_drifted` map keeps pointing at `refresh-listing`.

## 1.1.0

### Fixed

- A harvest that fails after its package is written prints its report instead of exiting with only the error of the step that failed: `written` true, `package`, the stage it stopped at and, in `next`, the resume and the undo; it still exits 1. Before, a test command whose database was down left the draft records and the package behind with the step's own "nothing was written".
- A harvest whose package directory holds something other than this spec's package, or holds the package of a finished harvest, is refused before any record is written or any counterfactual runs. Before, it wrote every draft record and then failed with "output directory is not empty".

### Changed

- Running the same harvest spec again over its own `draft` or `submitted` package resumes it (`resumed` true): a draft record that would be written unchanged is not written again (`record` `unchanged`, no audit event), a rule already in the package is not promoted again, the fill is written only when it differs, a submitted package is not submitted again, and a passing test result of the obligation's check and an approval by the same role and reviewer are kept. A submitted package whose rules changed since is refused, because its content is frozen.
- `stopped_at` tells the verified stages apart: `submitted` while test results are pending, `tested` once every result is recorded and the approval or verification is pending. A run without an approval stops at `tested`. The report always carries `written` and `resumed`.

### Added

- `discard-harvest --package-root <path> --registry-root <root> --repo-root <repo> --reason <text>` undoes a harvest that did not finish: it refuses anything but a draft or submitted package, retracts the package's draft records (leaving reviewed ones and drafts another package names), removes the package directory and appends one `discard-harvest` audit event naming what it retracted and kept.

## 1.0.0

### Changed

- A refusal prints `ERROR: <reason>` on stderr instead of stdout, with the same text and exit 1, so stdout carries only a command's output and a host no longer has to parse refusals out of the data. This covers the `ERROR:` lines of `validate`, `validate-policy`, `validate-change-package`, `verify-scm-attestation`, `verify-git-governance` and `recover-registry-update` too. The pre-commit hook no longer re-prints a captured `ERROR:` line, since it already reaches the terminal.
- Messages that told the user to run a removed command now name its replacement: "Apply each with ship-package", "run ship-package again", "run ship-package --commit <that commit>", "harvest --spec fills a draft", and the apply step's refusals read "ship-package refuses to review".

### Removed

- `fill-harvest-package`, `attest-signed-commit`, `finalize-proposal` and `apply-approved-updates`, legacy since 0.83.0, are gone. Use `harvest --spec <file>` for the first and `ship-package` (with `--commit <sha>` for a signed commit) for the other three. The names are now an unknown-command usage error. Audit events and `ship-package`'s step names keep the old names, because committed Registries hold those events and `verify-audit` reads them.

## 0.96.0

### Fixed

- `mcp-serve` puts the caching hints the 2026-07-28 revision requires, `ttlMs` 300000 and `cacheScope` `public`, on `server/discover` and `tools/list` results of a per-request session. A real host that discovers and lists without `initialize` rejected the list without them and loaded no tools; a session negotiated through `initialize` is unchanged.

## 0.95.0

### Fixed

- `mcp-serve` adds `resultType` to a result only when the request declared its protocol version in `_meta` (the 2026-07-28 per-request shape) or is `server/discover`. A session negotiated through `initialize` on 2024-11-05 to 2025-11-25 gets the plain result shape those revisions define; a strict client of 2025-06-18 read the stray `resultType` as the newer cacheable-result shape, demanded `ttlMs` and `cacheScope`, and loaded no tools.

## 0.94.0

### Changed

- The command handlers are split by theme into a `domain_registry/commands/` package (`read`, `evidence`, `lifecycle`, `governance`, plus the helpers two themes share in `common`), and `cli_parser.py` registers its commands in the same four themes; a test holds each parser theme to exactly the commands whose handlers its module holds. `cli_commands.py` stays as the façade that merges them, so nothing that imports it changed. Behaviour and `--help` output are unchanged; the only visible difference is the order of command names in argparse's "invalid choice" error. The two hook installers resolve `registry_tools.py` from their new depth, and a test runs both and checks the hook's `tool=` line.

## 0.93.0

### Changed

- The drift `NOTE:` of `verify-sources` says, when `content_changed` is non-empty, how many selected sources changed inside and that `verify-evidence` checks the citations, so nobody reads "no confirmation needed" as a claim that the confirmation covers the edited text. A confirmation covers which paths are sources; every cited span carries its own hash. Status and exit stay as they were, and the reference states that `refresh-listing` refuses only when the selection itself changed or the map is invalid or unverified.

### Fixed

- The pre-commit hook passes `--find-renames` to its staged diff, so the `relocate-evidence --from <old> --to <new>` hint prints whatever the user's `diff.renames` setting is; a rename was already refused either way.

## 0.92.0

### Fixed

- The pre-commit hook checks deletions and renames as well as additions and edits. Before, `git rm` of a cited file never reached `verify-evidence --staged`, and a rename was checked only under its new name, which nothing cites, so both commits went through and left every rule citing that file citing nothing. The hook now checks the old and new path of a rename and the path of a deletion, `verify-evidence --staged` says a missing citation's path "is not in this commit" and points at `relocate-evidence`, and the hook adds the exact `relocate-evidence --from <old> --to <new>` line for each rename. The reference also notes that a hook installed under a tracked `core.hooksPath` changes every clone's commits, so that install is the repository owner's decision.

## 0.91.0

### Added

- `refresh-listing` rewrites only the discovery listing a source map keeps (`source_groups` and the `missing_groups` derived from it) to what discovery finds today, so new files appear and vanished ones drop out without resetting a developer's confirmation. It refuses and writes nothing unless the selection itself verifies as `current`, prints `Listing already current` when nothing drifted, and otherwise records an `added`/`removed` audit event and prints `Listing refreshed: +N -M discovered paths`.

### Changed

- The `NOTE:` that `verify-sources` prints for a `discovery_drifted` map points at `refresh-listing (no confirmation needed)` instead of `refresh-sources` then `confirm-sources`, because the listing is not evidence and a new test file should not force a fresh confirmation; a `stale` map keeps the two-command advice.

## 0.90.0

### Changed

- `mcp-serve` checks a tool's required arguments before running the command. A missing one is reported as the JSON argument the host sent (`get_context: argument id is required`, plural and sorted when several are missing), not as a command-line flag. A command that succeeds and prints a JSON object also returns that object as `structuredContent` beside the unchanged text; the seven tools that always print an object declare an object `outputSchema`, so hosts on 2025-06-18 or later can read results without parsing text. The reference says the server keeps no session: it answers requests sent before `initialize` and accepts a repeated one, which the published lifecycle allows and whose refusal code the newest revision forbids.

## 0.89.0

### Fixed

- The pre-commit hook checks what the commit will contain. It runs `verify-evidence --staged`, which reads each cited file from the Git index, so a weakened line no longer slips through because the disk holds the original. `--staged` counts an index entry only at stage 0, as a regular file, at the exact path; anything else is `missing`, and an index Git cannot read is an error.
- The hook finds the hook it chains to through Git at run time (`git rev-parse --git-path hooks`), so it, and the pre-push hook's chained hook, run from a linked worktree too.
- A refusal names each record and cited span and asks you to check the statement before reciting; the full JSON report appears only with `DOMAIN_MEMORY_HOOK_VERBOSE` set, while an `ERROR:` line is always shown.
- Python starts only when a staged path appears as a citation or source path in the Registry's `registry/` directory or `source-map.json`, not when it is merely mentioned in a package's prose.

## 0.88.0

### Changed

- The command list printed by `--help` follows the three groups, Everyday, then Advanced, then Legacy, instead of the order the commands were registered in. Each legacy command's one-line help ends with `(legacy: use <replacement>)`, so it is marked on the first screen and not only in the epilog; the reference's legacy rows carry the same label.
- `init-domain-memory --output` says in its help that the directory must lie inside the repository, which the command has always required.
- The brief's source-map attention line counts removed files with an ASCII minus (`+110 -3`), not U+2212, so a consumer printing it on a narrow Windows console no longer fails.

## 0.87.0

### Fixed

- `brief --base` no longer asks a reviewer to place a test file that lies outside every Context when that file is only cited as a rule's proof. Each such file is still listed under `outside_every_context`, now with `cited_as`, the path's kind in the source map, and only files not of kind `tests` get the "place it or move the citation" line; an unknown kind keeps it.

### Added

- The change brief carries `source_map`: whether `source-map.json` changed inside the range, who confirmed it or that nobody did, and how many discovered files it gained and lost, as one attention line and a "Source map" section in the markdown. A re-snapshot that widens what counts as evidence can no longer pass review unseen.

## 0.86.0

### Changed

- `init-domain-memory` is about three times faster on a large repository with the same output. It walks the selected sources once per map instead of twice, stops asking git whether tracked files are ignored, since git never ignores them, and makes one file-system call per file instead of two. A test holds the walk count and the byte-identical output.
- `references/registry-schema.md` states that an Aggregate's `invariants` are prose and not a link to rules; a Rule's own `aggregate` field is that link.

## 0.85.0

### Fixed

- `verify-sources` reports `discovered_but_unlisted`, the paths discovery would place in a group today that the map does not list, beside `discovered_but_gone`. When either is non-empty, a map that would be `current` reports `discovery_drifted`, still with exit 0 because the listing is not evidence, with one `NOTE:` on stderr carrying both counts and pointing at `refresh-sources` then `confirm-sources`; `stale` is never replaced and the hash fields keep their own verdict.
- `refresh-sources` prints `Selection reset to agent-asserted; confirm these N selected sources: <paths>` instead of saying the sources require confirmation, because the paths are the selection whose confirmation it reset, not files that changed.

## 0.84.0

### Added

- `mcp-serve` runs a Model Context Protocol server on standard input and output, built on the standard library alone, and offers thirteen read commands as tools: `get-context`, `get-record`, `resolve-terms`, `lookup`, `dependents`, `coverage`, `brief`, `boundary-impact`, `verify-evidence`, `validate`, `readiness`, `list-proposals` and `validate-change-package`. No command that writes is offered, and a test holds the exposed set to a literal read-only list. The Registry and repository roots are the server's own arguments, never the client's; a path argument outside the repository is refused. Each tool runs the command's own handler, so its checks and NOTEs come back unchanged: stdout as the first text, a NOTE as the second, and a refusal or a wrong argument as a tool error the model can read. The server speaks the 2026-07-28 per-request protocol with `server/discover` and the older `initialize` handshake, answers `ping`, replies to a malformed line with a parse error and keeps serving, and exits 0 at end of input.

## 0.83.0

### Changed

- Commands are grouped into everyday, advanced and legacy in `references/script-api.md` and in `registry_tools.py --help`, from one table in the parser that a test keeps in step with the reference. `fill-harvest-package`, `attest-signed-commit`, `finalize-proposal` and `apply-approved-updates` are legacy, because `harvest` and `ship-package` run them; they still work, print a NOTE on stderr naming the replacement, and are removed in 1.0.0.
- `validate` and `validate-change-package` print a NOTE for a `consistency` other than `synchronous` or `eventual`, naming the record or package, without failing. A Contract's `kind` and an Interaction's `delivery` stay free prose; the schema page says why.

## 0.82.0

### Added

- `install-pre-commit-hook --registry-root <root> --repo-root <repo>` installs a POSIX `sh` pre-commit hook. It refuses a commit when a staged file's Registry citations are stale, missing, unhashed or invalid, naming those records and paths, and when the commit is allowed prints any `boundary-impact` verdict other than `no-cross-context-change` as a NOTE that never refuses. A commit that stages no file the Registry names runs no Python. The pre-push and pre-commit installers share one installer, one interpreter search and one way of chaining an existing hook; the generated pre-push hook is unchanged byte for byte. The hook reads files as they are on disk, not as staged, and does not check a deleted cited file.

## 0.81.0

### Changed

- `brief --base` counts in the singular when there is one ("1 rule cites", "1 stale citation"). It lists under `registry_changes` the records the range added, amended or removed, each with the proposal that last reviewed it, and gives one attention line per asset and proposal. Each listed rule carries the check that proves it and the directory to run it in, so a reviewer runs the named test instead of looking it up with `get-record`.

## 0.80.0

### Added

- `scripts/run_tests.py [--jobs N] [module ...]` runs each test module in its own interpreter, several at a time, and prints one line per module with its summary and the failing modules' output in full; the plugin's suite had grown to ten minutes when run in one process.

### Changed

- The four largest test files (review, updates, CLI, registry) are split into topic files of at most about 570 lines each; test names, bodies and count are unchanged, and the one helper two files share, `sign_a_commit_changing`, moved into the shared `RegistryTestCase`, so a failing test points at a small file named for what it covers.

## 0.79.0

### Changed

- `brief --base` lists every record, of any asset, whose cited lines the change touches, test citations included, each with the spans touched and the files whose cited excerpt changed, and adds one attention line counting the rules whose checks should run again. It used to list only collaboration records, so a change to the tests that prove fourteen rules showed an empty record list, and a weakened rule was reported as one stale citation to recite.
- The attention line for changed evidence says to check that the record's statement still holds before reciting, and quotes the statement; the review-change capability says the same: `recite` is never the default answer to a stale citation.
- `ship-package` on an already applied package says in `next` that there is nothing to do but commit what may still be uncommitted, and a refused `--commit` is echoed resolved in full beside the attested one.

## 0.78.0

### Added

- The `domain-memory-review-change` capability: reviewing a pull request or a branch through the model, from one `brief --base <branch point>`, with what each verdict asks of the author, how the records to re-read and their dependents are used, what stale citations mean for the review, and the attention list as the review's checklist.

## 0.77.0

### Added

- `brief --base <revision>` and `brief --asset <asset> --id <id>` compose one document to read before a review or a change: the boundary verdict, each Context touched with its changed files and summarized model, each collaboration record whose cited lines changed or overlap the change with the records that depend on it, the citation verification of the changed files, the cited files outside every Context, and an `attention` list with one line per thing to act on; for a record, its Contexts, dependents, verified citations and attention. `--markdown` renders it for a pull request.

### Changed

- `ship-package` can be run again: an `approved` proposal is only applied, an `applied` one reports `note` "already applied; nothing was written" with exit 0, and a `--commit` other than the attested one is refused before anything is written.
- `harvest` names a faulty rule by its id, "rule <id>: statement must be text", and by its position, "rule 2", only when it has no id.

## 0.76.0

### Added

- `ship-package --package-root <path> --registry-root <root> --repo-root <repo> [--commit <sha>]` runs the tail of a review in one call: `attest-signed-commit` for the given commit, then `finalize-proposal` and `apply-approved-updates`, stopping at the first step that refuses and naming it. Without `--commit` it starts at `finalize-proposal`, for an attestation written by hand. The report lists the steps completed, the proposal's status afterwards and the next step; it writes the package and the Registry, never git.

### Changed

- `harvest` reports a refused spec or citation as it reports every other stop: `stopped_at` is `spec` or `citation`, `errors` names each gap or path, and `next` says what to do; citations are checked before any counterfactual runs. `package` is null whenever the run stopped before writing. A reviewed rule whose statement, examples, contexts and cited spans are unchanged is reported `reproven` rather than `amended`, so a reviewer knows the package carries fresh proof and no new content.

## 0.75.0

### Added

- `harvest --spec <file> [--through draft|verified]` runs the harvest loop from one spec: every counterfactual first, as one suite, writing nothing unless each is killed; then citations of the enforcing lines and the proving test, a candidate per rule (a reviewed rule is amended and its reviewer superseded), the Change Package, its promotion and fill, and a `--as-status verified` preview; `--through verified` adds submission, each obligation's test result, the approval and verification. The report names each rule's verdict and record outcome, where the run stopped and what comes next; the signed commit stays with the signer.

## 0.74.0

### Added

- `counterfactual --suite <json file>` judges several mutations in one call: one intact run per test command, each file restored before the next mutation, an entry's refusal recorded as its own `error` result, a `summary` of the verdicts, and the worst verdict as the exit code.

### Changed

- `counterfactual` refuses an intact run that passes while running no test, `go test` without `-v` or `-json`, a unittest run of 0 tests or every test skipped, a pytest run in which nothing passed, and judges such a run on the broken file `inconclusive`; the same reading `record-test-result` already applied. A harness that ran `go test` without `-v` must add it.

## 0.73.0

### Changed

- The NOTE on a counterfactual that mutates an uncited file names the rule it proves, not the obligation and the rule side by side.

## 0.72.0

### Added

- `validate-change-package` prints a NOTE when a counterfactual mutates a file that the rules its obligation proves do not cite; the pattern verification reference states when that split is right, with the Workspace-scoping case as the example: weaken the predicate in the query a cited call runs rather than the id the call passes, so only a stranger's rows leak.

## 0.71.0

### Changed

- `separate-ways` is no longer an Interaction `relationship`: an Interaction records that two Contexts collaborate, and separate ways is the decision not to, so `validate` refuses it and says to record a dependency policy with `policy: forbidden` and `mode: none`.
- What makes a change material is defined once, in the seven-step workflow; the root skill, the design and hygiene skills, the host integration page and the brownfield fast path refer to that definition instead of carrying five differing lists.
- A dependency policy is "allowed or forbidden" everywhere, matching the value `validate` accepts; "prohibited" named the same thing on two pages.

## 0.70.0

### Changed

- AGENTS.md names all four record statuses, the root skill description names extraction and implementation hygiene among the capabilities it routes to, and the extraction reference's asset table has a Capability row, so the three agree with the schema and the skills that exist.

## 0.69.0

### Added

- An Interaction may name the Context Map pattern between its two Contexts in `relationship`: `partnership`, `shared-kernel`, `customer-supplier`, `conformist`, `anticorruption-layer`, `open-host-service`, `published-language` or `separate-ways`, the producer being upstream; `validate` refuses any other value.
- `validate` refuses a dependency policy whose `mode` or `policy` is outside the documented set.
- `dependents` given an unknown id names up to three ids that start with or resemble it, as `get-record` does.

### Changed

- `shared-kernel` is no longer a Context `subdomain`: a shared kernel is a relationship between two Contexts, and the refusal says to record it on their Interaction. A Registry that used it moves the value to the Interaction; this repository's Registry never did.

## 0.68.0

### Added

- Rules and Events may name the Aggregate whose invariant they state or raise in `aggregate`, and an Event the Contract it is published under in `contract_id`; `validate` refuses an unknown Aggregate or Contract and an Aggregate that sits in a Context the record does not name, and the concept file links them under Relations.
- `relocate-evidence` prints a NOTE when `--to` lies over no file in the repository and a Context path was proposed there, so a typo surfaces before a package is built.
- `dependents --asset <asset> --id <id>` walks every relation from a record in both directions, without crossing a Context, and lists what a change to it may touch.

### Fixed

- `lookup` summaries and `get-context` summaries read one field list; the two definitions that drifted apart are one.

## 0.67.0

### Fixed

- `relocate-evidence --from <repository path>` now also moves a Context whose `implementation_path` ends that path the way `boundary-impact` maps files to it, so one call re-cites the evidence and relocates the Context; before, only the Registry's own form of the path moved the Context, and that form never matched a citation.

### Added

- `relocate-evidence` prints `citations_in_place`, the citations under `from` whose lines stayed in their own file, and a NOTE when nothing needed moving, telling "already fine" apart from a path in the wrong form. The `validate` hint names the repository path form.

## 0.66.0

### Changed

- `lookup` and `resolve-terms` stem each distinct word once and test a query word's forms as one set, `get-record` and `get-context` read a record from its own concept file, and `coverage` and `get-context` read each asset once per call. Results are unchanged; on a 10,000-rule Registry a lookup takes a third of the time and `get-record` reads one file instead of ten thousand.

## 0.65.0

### Changed

- `relocate-evidence` gives a Context whose `implementation_path` is `--from` or lies under it the new path, in the same output, so a move no longer leaves `boundary-impact` mapping the moved code to no Context.
- `validate --repo-root` refuses a reviewed Context whose `implementation_path` lies over no file in the repository, by the same test `boundary-impact` uses, and notes a candidate's, which may name code to come.
- The brownfield refactoring fast path lists the re-citation commands, and the Maintain and Extract Skills link the Registry schema and that path, which the root page alone linked before.

## 0.64.0

### Changed

- `analyze-boundary` reports each direction on its own, `source_depends_on_target` and `target_depends_on_source`, each with interactions, contracts, dependency policies and consumed events. It filtered interactions by producer and policies by depender under the same `--source-context`, so a Context that calls another and may also be depended on by it came back as one relation, and the reverse question came back as `no_registered_collaboration`.
- An Event's `consumers` are validated as existing Context ids, count as a collaboration in `boundary-impact`, `change-coupling` and `analyze-boundary`, and put the event in the consuming Context's `get-context` model; `resolve-terms` names them among a mention's Contexts.

## 0.63.0

### Changed

- A usage mistake in `counterfactual`, such as an unknown option or a missing `--file`, prints JSON with `verdict` `error` and exits 3, as its other refusals do; it exited 2, the exit code of `inconclusive`. Other commands keep argparse's message on stderr and exit 2.
- A `--find` text that occurs more than once lists its first ten lines and how many more.

## 0.62.0

### Changed

- `counterfactual` prints every refusal as JSON with `verdict` `error` and exits 3; a refusal printed plain text and exited 1, the exit code of `survived`.
- When the intact run fails, the refusal carries that run's failure lines with its tail, its `failing_tests`, and `log_path`, a file holding the whole output, instead of the last 1200 characters alone.

## 0.61.0

### Added

- `supersede-proposal --superseded-by <id>` notes when no package beside it carries that id yet, and `validate` and `verify-git-governance` name each superseded package whose replacement no package under `changes/` carries, so a mistyped id is seen.

## 0.60.0

### Changed

- `supersede-proposal` and `redraft-proposal` accept, and do not use, `--registry-root` and `--repo-root`, as the other lifecycle commands take them.
- `supersede-proposal --superseded-by <id>` says the proposal was superseded by that id instead of asking for a new draft.

## 0.59.0

### Changed

- `submit-proposal` refuses a promoted record citing a path outside the confirmed sources, and `validate-change-package`, `verify-proposal` and `finalize-proposal` report it, so the refusal that only `apply-approved-updates` gave comes before approval and the signed commit.
- `validate` and `verify-git-governance` print a NOTE naming the approved Change Packages not applied yet, with how to retire one that cannot be.
- `counterfactual` names the lines of a `--find` text that occurs more than once.

## 0.58.0

### Added

- `coverage` prints `rule_totals`: reviewed and candidate rules, Contexts without a reviewed rule, and, with Change Packages, how many reviewed rules are proven, unproven or unknown, each rule counted once.
- `verify-git-governance --stdin` checks every commit read from standard input and prints one line for them all.

### Changed

- The pre-push hook verifies a push's Registry commits in one call, so a push of many signed commits prints one line, and a failing `git rev-list` stops the push instead of passing it unchecked. Reinstall it with `install-git-hitl-hook`.
- A survived counterfactual's reason says to rerun over the whole suite when the test command selects only some tests, where a kill names the protecting test in `failing_tests`.

## 0.57.0

### Added

- `promote-candidate --cite <path>:<start>-<end>` and `--cite-function <path>:<name>`, with `--repo-root`, add citations to the promoted record's evidence, each once, so re-proving a reviewed rule is done from the command line.

## 0.56.0

### Added

- `promote-candidate --amend` copies a reviewed record into a draft without its review fields and supersedes the proposal that reviewed it, so a rule can be re-proven.

### Changed

- `failing_tests` names a Go subtest by its top-level test, once.
- `fill-harvest-package` says that `submit-proposal` validates the package it filled.
- The harvest recipe ends with a merge from upstream, never a rebase, and says how a reviewed rule is re-proven and how a rule a dependency gate enforces is proven.

## 0.55.0

### Added

- `fill-harvest-package` fills a draft package that promotes harvested rules from a spec: one acceptance criterion and one test obligation per rule, the rule ids, contexts, proof obligations, the files the rules cite as sources, and the list of counterfactual checks. It refuses a rule whose counterfactual was not killed, ran another command than the rule's check, or names another obligation, and `--dry-run` prints the documents and refuses alike.

## 0.54.0

### Added

- `counterfactual` names in `failing_tests` the tests that failed on the broken file, read from Go, pytest and unittest output, and the extract capability finds a rule's proving test that way instead of by text search.

### Changed

- `coverage` lists a Context's reviewed rules by proof in `proven`, `unproven` and `unknown`, each a list of ids, and accepts `--repo-root`.

### Fixed

- A rule re-cited by a later package without test obligations kept its proof: `get-record` and `coverage` now take the proof from the most recent reviewing package that covers the rule, and a NOTE names the packages applied since; a reviewing package that is not there makes the proof unknown, not absent.

## 0.53.0

### Added

- `coverage` lists in `rules_by_context` the reviewed and candidate rules of every Context, fewest reviewed first, and with Change Packages the reviewed rules no test obligation proves.
- The workflow reference gives the order of a rule harvest: one counterfactual per rule, then the five signing steps as one block.

### Changed

- `get-record` reads Change Packages from the Registry's `changes/` directory when no `--changes-root` is given, and adds `proven_by` only to rules and contracts.
- A `proven_by` entry names the `working_directory` its check passed in.
- A `resolve-terms` mention takes its `summary` from the asset's prose field, such as a dependency policy's `rationale`.

## 0.52.0

### Added

- `get-record --changes-root` adds `proven_by` to a reviewed rule or contract: the test obligations of the package that reviewed it, with their checks.
- `cite --function <name>` cites the whole body of a Go `func` or Python `def`.

### Changed

- A word in a search matches its spaced, hyphenated and camel-case forms, and a participle in `-en` is a form of its verb, so `takedown` finds `taken down`, `take-down` and `TakedownAt`.
- A `resolve-terms` mention names its `status`, `contexts` and the first sentence of its text as `summary`.
- `submit-proposal` refuses an obligation whose check runs `go test` without `-v` or `-json`.

### Removed

- The `authoritative` field of a `resolve-terms` mention, which was always false; `match_kind: "mentioned"` already says the record does not define the term.

## 0.51.0

### Added

- `counterfactual_check` may be a list of checks, each targeting a different obligation, when one package proves several protections.

### Changed

- `counterfactual` calls broken code that does not build `inconclusive` even when `--expect` matches its output, and refuses a verdict word such as `FAIL` or `passed` as `--expect`.
- `record-test-result` runs the obligation's `check` when given no `--command`, and refuses a command other than that check; surrounding whitespace is not recorded.
- `record-test-result` records only while the proposal is draft or submitted.
- `verify-proposal` refuses a test result whose `evidence` is a command other than its obligation's `check`; an `https://` link to a CI run is still accepted.
- `get-context` and `get-record` suggest the ids that start with the one given before the nearest by spelling, so a truncated id finds its record.
- `--source-authority` and `--command-profile` describe themselves in `--help`, as `--expect` and `--command` do.
- `record-test-result` refuses a run that exits 0 without passing a test: `go test` without `-v` or `-json`, or one in which every test was skipped; `[no tests to run]`; unittest that ran nothing or skipped everything; a pytest summary with nothing passed.

## 0.50.0

### Added

- Every command has a one-line help, the task it serves in the script reference, and `--help` lists them.
- `cite` takes `--lines START-END` as `recite` does, beside `--start` and `--end`.
- `get-context` and `get-record` name up to three nearest ids when the one asked for does not exist.
- `--asset` takes the singular, such as `rule` or `dependency-policy`.

### Changed

- `get-context --summary` keeps the Context's `implementation_path`.
- `cite --show` marks the lines it leaves out between the first and the last.
- Output ends lines with a line feed on every platform, and the `upsert-candidate` NOTEs all go to stderr; the reference states the output convention of every command.
- An empty `--query` is a usage error.
- A root that holds no Registry is named as such, and one pointing at `registry/` itself says to pass the directory containing it, for the concept layout as for JSON.
- An unknown Context is named in the error, with the advice to add it first.
- `upsert-candidate` says when a record carried a review status it does not keep.
- `init-domain-memory` names the `--source-authority` it was given instead of a developer.
- The extract capability describes the scratch root, `compare-registry` and `harvest-candidates` flow.

## 0.49.0

### Added

- `quality-gates` reports a Go module with tests as a `go-test` gate and each Taskfile `lint`, `typecheck` or `test` task as a gate, and gives each pipeline gate its path.
- `list-proposals` lists in `supersedes_unknown` the ids a package supersedes that no package carries.

### Changed

- `lookup` no longer ranks records that share with the query only words many records hold, such as `skill`; it lists nothing and says so. Question words such as `who`, `can` or `what` are no longer searched.
- A noun in `-ure` is the same word as its verb forms, so `exposure` finds `exposed` and `failure` finds `failed`.

## 0.48.1

### Fixed

- An `also_covers` entry naming an acceptance criterion, rule or contract the package does not name is refused by name, where a typo used to read as if the field were ignored.

## 0.48.0

### Added

- A test obligation can list in `also_covers` further acceptance criteria, rules or contracts its check proves, each as `{source_type, source_id}`, so one check covering several sources is recorded and run once instead of repeated per source.

### Changed

- The `change-coupling` reference says that a fan-in feature crosses `--max-modules` like a sweep, and how to tell them apart.

## 0.47.1

### Fixed

- `change-coupling --max-changeset` counts only the files under the roots, so a feature commit that also ships web, contract or migration files is no longer skipped as large; `skipped_large` shows `files_under_roots` beside `files`.

## 0.47.0

### Added

- `change-coupling` lists the newest twenty commits it skipped in `skipped_large` and `skipped_sweeping`, with their sha, subject and the files, modules or Contexts that made them skip, so a reader can check that each is a sweep.

## 0.46.0

### Added

- `boundary-impact` lists in `boundary_evidence_touched` the interactions, contracts and dependency policies whose cited lines overlap a line the change added or rewrote.

### Fixed

- `compare-registry --context` pairs every Context before scoping, so an interaction or dependency policy between a scoped Context and one outside the scope is matched instead of counted as both missed and extra.
- `change-coupling --max-modules` with `--registry-root` counts only modules a Context owns, so feature commits that also touch wiring or generated code are no longer skipped as sweeps.

## 0.45.1

### Fixed

- `counterfactual` evidence keeps with each failure line up to three lines indented under it, so the assertion message of a failure outside the tail is no longer dropped.

## 0.45.0

### Changed

- Every command writes UTF-8 to stdout and stderr, also when piped on a Windows console whose code page is not UTF-8, so a consumer reading JSON as UTF-8 no longer fails on a Registry holding non-ASCII text.
- `counterfactual` names its evidence `failure_evidence` at the top level as in `counterfactual_check`, in place of `failing_evidence`, and starts it with the output's failure lines, such as `--- FAIL:` or `panic:`, before the last part of the output.
- `init-change-package --recitation` says when a record adds or drops a citation and changes nothing else, and otherwise names the fields it changes.
- `validate-change-package --as-status submitted` or `verified` notes that approvals are not checked only when the package still lacks one.

## 0.44.0

### Changed

- A `resolve-terms` mention lists each record once with every field that mentions the query as `fields`, in place of one mention per `field`.
- `lookup` and mention matching no longer count words such as `after`, `the` or `when` as shared, so a record holding only those no longer ranks.

## 0.43.0

### Added

- `get-context --summary` gives each record as its id, status and one line, and every Context model lists `rules_bound_through_collaborations`, rules of other Contexts citing a file one of its interactions or contracts cites.

### Changed

- `lookup` falls back from the query as written to every word of it, in any order and in another form of the word, then to the records holding most of its words, and says on stderr when it did, or that finding nothing is not proof that no constraint applies.
- `resolve-terms` finds a mention holding every word of a multi-word query, lists rules and contracts first among mentions, and notes when the result limit cut matches or a matching term hid rules that mention the query.

## 0.42.2

### Fixed

- The message for a Registry held open no longer says the update could not be swapped *in* when it failed while moving the Registry aside.

## 0.42.1

### Fixed

- A Registry another process holds open no longer leaves an interrupted transaction behind when the update cannot move it aside, and the refusal says in English that nothing was changed and what to close, instead of the operating system's message in the console code page.

## 0.42.0

### Added

- `harvest-candidates` lists every Context pairing in `context_pairs` with how it paired, names a Context sharing an id here without sharing its path in `context_id_collisions`, and skips a record citing exactly a reviewed record's excerpts as `skipped_duplicate` unless `--keep-duplicates`.

### Changed

- `verify-audit --against` fails when no audit manifest is committed at that revision instead of passing with nothing compared.

### Fixed

- A Registry update whose directory swap Windows refused for a moment, after the Registry had been moved aside, left no Registry and a journal whose recovery deleted the moved copy. Every rename in an update now retries a refused replace, a swap that still fails puts the Registry back at once, and recovery restores a Registry missing after a prepared swap instead of discarding the only copy.

## 0.41.0

### Added

- `harvest-candidates --from-root` writes a scratch extraction into the Registry as candidates in one update, under the Contexts its own Contexts pair with, skipping records reviewed here.
- `verify-audit --changes-root` requires each applied proposal's `apply-approved-updates` event, and `--against <revision>` requires the log to extend the one committed there, so a consistent truncation of the newest events is caught.

### Changed

- `upsert-candidate` notes a possible duplicate when one record's cited excerpts are all among the other's, the test `copied_evidence` uses, not only when they are identical.

## 0.40.0

### Added

- `upsert-candidate` says whether it created or updated each record and notes a record citing exactly the excerpts of another as a possible duplicate.
- `quality-gates` reports a hook the repository ships but Git does not yet run, such as `.githooks/pre-push`, with `enabled: false`.

### Changed

- `readiness` reports the verifier and trigger the policy configures instead of recommending one.

### Fixed

- `relocate-evidence` no longer looks in the new file for a citation whose lines are still in its own file, so splitting a file no longer reports the lines that stayed as unmatched and blocks the re-citation package.

## 0.39.0

### Added

- `boundary-impact` reports `mapped_files`, how many changed files the verdict speaks for, and `cited_outside_contexts`, the changed files no Context owns that a Registry record cites, with the citing records.
- `list-proposals` flags packages sharing a proposal id.

### Fixed

- Writing a JSON document retries a replace Windows refuses for a moment, when an indexer or virus scanner holds the new file, and removes its temporary file if the replace never succeeds.
- `init-change-package` and `redraft-proposal` refuse a proposal id a package beside the output already carries, and remove a package they fail to finish instead of leaving one that reads as valid.

## 0.38.0

### Added

- `compare-registry` gives each `overall_*` score its `matched`, `gold` and `candidate` counts.
- `redraft-proposal` lists the superseded proposal in the new draft's `supersedes` and records `redrafted_at`.
- `list-proposals` shows a superseded proposal's `superseded_by`, flagging one that names no package, and counts redrafting and superseding in `last_changed_at`.
- A failing `verify-evidence` names the failing counts on stderr.

### Changed

- `copied_evidence` no longer marks Context pairs: both sides naturally cite the package documentation, and Contexts already pair by path.

## 0.37.0

### Added

- `compare-registry` reports `overall_independent`, the attempted score with every `copied_evidence` pair left out of both sides, and `copied_matches`; the command notes when copied pairs are more than half the matches.

### Changed

- A `survived` `counterfactual` verdict's reason names an equivalent mutation as a possible cause besides a missing test.

## 0.36.0

### Changed

- `change-coupling` reports `explained_by: null` and names the `unmapped` module when a module lies in no Context, so an empty `explained_by` always means coupling the Registry does not account for.
- `compare-registry --context` counts only candidate records of the candidate Contexts paired with the scoped ones, and `overall_attempted` leaves out assets either Registry holds no records of, so it is `null` when nothing in scope was extracted.
- `copied_evidence` also marks a pair when one side's cited excerpts are all among the other's, the shape a harvest that dropped a citation leaves.
- Every `counterfactual` verdict carries a `reason`.

## 0.35.0

### Added

- `change-coupling --max-modules <n>` skips a commit touching more modules than that and counts it in `skipped_sweeping_commits`; with `--registry-root`, each pair lists `explained_by`, the Context or the registered interactions, contracts and dependency policies that account for it.
- `compare-registry` reports `overall_attempted`, the score over the assets the candidate extracted, marks a pair citing exactly the gold excerpts as `copied_evidence`, and with `--context` reports `scope.without_candidate`.

### Changed

- `compare-registry --context` refuses a Context the gold Registry lacks instead of scoring nothing, and notes a scoped Context no candidate Context pairs with.
- The `mutated_protection` a killed `counterfactual` prints is one line, whatever whitespace the find and replace text holds.

## 0.34.0

### Added

- `counterfactual --expect <regex>` counts a failure as `killed` only when its output matches, `--cwd <directory>` runs the command from a directory inside the repository, and `--obligation-id <id>` fills the `counterfactual_check` a killed run now prints, the object a Change Package's evidence records.
- `resolve-terms --with-mentions` lists the reviewed records mentioning a query after the terms that match it.
- `compare-registry --context <gold id>` scores an extraction against one part of the gold Registry, and each asset lists `possible_pairs`: missed gold records worded like an extra candidate in a paired Context.
- `change-coupling --exclude-subject <regex>` leaves out sweeping commits by their subject, each pair lists the subjects of its shared commits, and `--registry-root` names the Context of each module.
- `verify-evidence --strict` also fails on a shifted citation.
- `cite --show` prints the first and last cited lines.
- `list-proposals --changes-root <dir>` lists every Change Package with its status, last change, number of updates and supersedes.
- `coverage` counts events and takes a confirmed absence of events.

### Changed

- `counterfactual` runs its command without a shell, like `record-test-result`, and refuses to start while any `.counterfactual-original` copy is left anywhere in the repository, not only beside the file it breaks.
- `counterfactual` reports a broken file that does not build, such as an unused variable or a syntax error, as `inconclusive` with a `reason`, not `killed`.
- `analyze-boundary` reports `policy_only` when only a dependency policy connects two Contexts.
- `boundary-impact` no longer counts a citation whose lines only moved within a changed file as changed boundary evidence.
- `compare-registry` pairs Contexts whose `implementation_path` one Registry writes from the repository root and the other from a module, and pairs records citing the same file by how many cited lines they share, so the closest record wins.
- A citation inside a selected source that no discovered group classifies is `selected`, not `unclassified`, so a record citing it can be reviewed.
- `quality-gates` counts golangci-lint `depguard` settings as an architecture gate and each Git hook as a hook gate, wherever `core.hooksPath` points.
- `readiness` routes a repository with code and an empty Registry to `extract`.
- `promote-candidate` adds the Contexts a record names to the proposal's `affected_contexts`.
- A re-citation package's obligation carries the full `verify-evidence` command, ready for `record-test-result`.
- `validate-change-package --repo-root` refuses decision evidence naming a file that does not exist, and notes a package that changes no record.
- Messages name a record as `<asset>/<id>` instead of the file name the Registry no longer uses.

### Fixed

- A command no longer crashes printing text the console's encoding cannot show, and `counterfactual` reads test output in the system encoding when it is not UTF-8.
- `counterfactual` retries restoring a file the system briefly refuses to write, and names the copy to restore from when it cannot.
- `upsert-candidate` names a record file that does not exist instead of calling it invalid JSON, and says how many records it wrote.

## 0.33.1

### Fixed

- `validate`, `verify-evidence` and `change-coupling` run in about half the time on a large Registry or history, and `compare-registry` in about a third, with the same output. A command now reads each cited file and works out each record's names, Contexts and cited paths once, and finds a citation's source kind by looking up its parent directories instead of scanning every source.
- `scan-secrets`, `discover-sources` and `verify-sources` skip excluded directories instead of walking through them, and `scan-secrets` reports a symbolic link it skips. Findings come in directory-walk order.

## 0.33.0

### Added

- The `domain-memory-extract` Skill turns a requirement, existing code, or a requirement change or refactoring into Registry candidates. It decides each element by its DDD definition, extracts language before boundaries and boundaries before tactical facts, and keeps how a finding was made out of the record. [Extraction](references/extraction.md) has the definitions and the three entries.
- `change-coupling --repo-root <repo>` reports which modules change together in the history, with the shared commits and degree of each pair, as one signal for where Bounded Contexts lie. It works from Git alone, in any language.

### Fixed

- [Evidence rules](references/evidence-rules.md) no longer describes the source kind and whole-file digest that citations stopped storing in 0.31.0.

## 0.32.0

### Added

- `compare-registry --gold-root <root> --candidate-root <root>` scores an extracted Registry against a reviewed one, per asset and overall, with the records it paired, missed and found extra. With `--gold-commit <sha> --repo-root <repo>` the reviewed Registry is read as that commit holds it, so a repository's history is its gold set.

## 0.31.0

### Changed

- A reviewed record carries `verified`, the OKF list of who approved it and when, instead of a `review` block repeating the proposal and its approvals. The proposal that reviewed each record is recorded in the audit log when it is applied, and `supersedes` checks, `recite` and `relocate-evidence` read it from there.
- A stored citation keeps its path, lines and `excerpt_sha256` only. Its `content_sha256` was never checked, and its source kind is derived from `source-map.json` whenever the citation is verified.
- `migrate-registry` refines a Registry written by an earlier version in the same way, recording each record's reviewing proposal in its audit event, and `validate` names a record still holding a `review` block.

## 0.30.0

### Added

- `export --commit <sha> --repo-root <repo>` prints the Registry as that commit holds it, in either layout, so a tool checking a push reads the pushed facts rather than the working tree's.

## 0.29.0

### Changed

- The Registry is stored as one Markdown concept document per record, in the Open Knowledge Format: `registry/<asset>/<id>.md`, with an `index.md` for the Registry and for each asset. A record's references to other records are typed links under `# Relations`, so people, other OKF tools and the scripts read the same graph. `init` creates this layout. A Registry kept as JSON files still reads and writes as before until `migrate-registry` moves it, which refuses, changing nothing, unless every record reads back unchanged. [Registry schema](references/registry-schema.md) describes the format.
- The Registry digest no longer depends on the order records were written in, so the same records give the same digest in either layout.

## 0.28.0

### Added

- `export --registry-root <root>` prints every asset document as one JSON object keyed by asset name (`contexts`, `rules`, `dependency-policies`, ...), each in the shape the asset's document has today. Tools outside the plugin read the Registry through it instead of opening its files, so the on-disk layout can change without breaking them.

## 0.27.2

### Fixed

- The installed pre-push hook stops the push when the hook it chains to (`pre-push.domain-memory-existing`) exists but is not executable, and says how to fix it. Before, it skipped that hook without a word, so a checkout, archive or sync tool that dropped the executable bit turned off whatever checks the earlier hook ran. Hooks installed before this release keep the old text until they are installed again.

## 0.27.1

### Fixed

- A signing program that will not start is recognized on Linux and macOS too, where Git says `cannot run <program>`; 0.27.0 knew only the Windows wording, `cannot spawn <program>`, and reported the signature as not verifying there.

## 0.27.0

### Changed

- A material proposal's `implementation_design` needs at least one entry in `domain_forces`, `invariants_preserved` and `rejected_alternatives`, as it already needed one proof obligation. An empty list used to count as complete, so a design with no forces and no alternative considered passed. A routine proposal may still leave them empty.

### Fixed

- External SCM verification reads every page of a pull request's reviews and of a commit's check runs, following the `Link` header 100 at a time. Before, it read only the first page, so an approval withdrawn later in a long review, or a failing check past the first page, went unseen. A page that is not a list is reported as a failed verification.
- A signature that could not be checked because the signing program would not start (`cannot spawn ssh-keygen`) is reported as not checked, with how to point Git at the program, instead of as a signature that does not verify. A signature that does not verify now carries Git's reason.

## 0.26.1

### Fixed

- Paths and Git output that are not ASCII are read as UTF-8 whatever the console's code page, so a non-ASCII source path is no longer dropped from discovery or misread as ignored.
- `quality-gates` skips a `package.json` or `pyproject.toml` that is not UTF-8 instead of crashing, and a `package.json` whose `scripts` is not an object is not reported as a gate.

### Changed

- `readiness` and `quality-gates` ask Git which candidates are ignored in one call instead of one per group, and walk the tree with string paths; both run about a fifth faster with the same output.

## 0.26.0

### Added

- `init-change-package --recitation <file>` turns what `recite` or `relocate-evidence` printed into a complete routine Change Package: the updates and supersedes, the Contexts the records name, a requirement that every citation is current, and one obligation checked by `verify-evidence`. It refuses anything beyond a re-citation, so a package it writes never claims less than it changes. A pure citation refresh no longer means filling the templates by hand.

## 0.25.1

### Fixed

- A commit whose signature does not verify is named, and the reason says which case it is: a commit that carries no signature, as a rebase, amend or cherry-pick leaves it, is told to take in upstream work with a merge, which keeps signed and attested commits; a signed commit whose signature fails says so. Before, both read only "Git commit signature is invalid". `AGENTS.md` adds the merge-not-rebase rule to the mistakes that cost a redo.

## 0.25.0

### Added

- `verify-sources` reports `discovered_but_gone`: entries of the stored discovery listing that no longer exist. The listing only tags citations with a source kind and never decides the status, so a renamed test file no longer sits in it looking like evidence while the map reads `current`.

## 0.24.0

### Changed

- `recite` takes `--lines` as optional: without it, the lines `--replace` names are re-cited, when they are in the file `--path` names. Code that changed but kept its lines is re-cited with one range instead of the same range twice.

## 0.23.0

### Added

- `recite --onto <file>` re-cites a record on top of what an earlier `relocate-evidence` or `recite` printed, and prints that output with the record's entry replaced, the `supersedes` merged and the resolved `unmatched` item dropped. A record with one shifted and one stale citation no longer comes out as two updates where the second overwrites the first.

### Changed

- A `shifted` citation is described as what it always was: it passes every check and needs no Change Package of its own; re-cite it in the next package that touches the record. `AGENTS.md`, `evidence-rules.md` and the `shifted` reason say so.

## 0.22.0

### Added

- `cite` and `recite` print a NOTE on stderr when the cited text also occurs elsewhere in the file, naming where, so an ambiguous citation is caught when it is written instead of going `stale` at the next edit.
- `init-change-package` without identifiers prints the flags still blank.

### Changed

- A `stale` citation whose excerpt repeats, and an `unmatched` relocation that found it more than once, say to widen the citation to the enclosing block.
- A test command that cannot start says that it runs as written, with no shell and no aliases, and shows a full invocation.

## 0.21.7

### Changed

- `references/registry-authoring.md` says how to choose between an Interaction and a Dependency policy: a host may enforce an `allowed` policy as permission to import, so a collaboration that avoids an import on purpose, such as a composition-root callback, an injected dependency or an event, is an Interaction.

## 0.21.6

### Changed

- `AGENTS.md` opens with how the parts fit together: what lives under the Registry root, the three paths a fact can take (read, candidate, review) with the review commands in order, the one route every write takes, and the four checks that keep facts honest.
- `README.md` maps the code for anyone changing the plugin: the entry point, which module owns what, and the rules that keep that shape.

## 0.21.5

### Changed

- `references/script-api.md` states what the repository walks skip: dependency, build and version-control directories, paths Git ignores, nested checkouts, symbolic links in snapshots, and fixture directories in `quality-gates`. Only names inside the repository count.

## 0.21.4

### Changed

- `README.md` names the test file for each area, so a change to one module can be checked in seconds before the whole suite runs.

## 0.21.3

### Fixed

- The rules for a registry digest, a commit id and a UTC timestamp, the walk over every `evidence` list, and the "read the policy if there is one" step each live in one place; the copies that restated them are gone, with no change to what any command prints or writes.
- Two helpers only the tests used, `security.scan` and `audit.append`, moved into the tests.

## 0.21.2

### Fixed

- `readiness`, `discover-sources` and `quality-gates` walk the repository once and skip dependency directories and nested checkouts without descending into them: on a large repository they drop from about 25 seconds to about 2.
- `audit-attestations` reads every attested commit through one Git process instead of two per package.
- `verify-sources` resolves each selected path once instead of once per file.
- A repository checked out under a directory named like an excluded one (`build`, `dist`, `vendor` and the rest) no longer loses all its source files: only directories inside the repository are excluded.
- Every command starts faster: the network modules load only when a GitHub attestation is checked.
- Every JSON document the plugin writes goes through one atomic write, so an interrupted write never leaves a half-written policy, source map or manifest.

## 0.21.1

### Fixed

- The pre-push hook no longer calls `python` by that name, which most Linux systems do not provide. It runs the first of `python3` and `python` that is Python 3.10 or later, looks for one only when the push carries a Domain Memory commit, and names a missing interpreter instead of failing as an unsigned commit would. Reinstall it with `install-git-hitl-hook`.
- `AGENTS.md` says which interpreter name to use on Linux, macOS and Windows.

## 0.21.0

### Added

- `init-change-package --proposer` fills the proposer, as `--requirement-id` and `--proposal-id` fill theirs.

### Changed

- A `shifted` citation's reason says that a reviewed record's re-cite goes through a Change Package listing the printed `supersedes`.

## 0.20.1

### Fixed

- A citation whose range now runs past the end of a shrunken file is searched for by its excerpt and reported `shifted` or `stale`, instead of `invalid`, which failed `validate` for the whole Registry.
- A `stale` citation and an `unmatched` relocation say why: the excerpt is gone or repeated, and what to do next.
- `relocate-evidence --from . --to .` covers the whole repository.

## 0.20.0

### Changed

- `resolve-terms` matches a mention as a whole word or its plural, so `check` no longer finds `checkpoints`, and lists mentions in a name, responsibility, statement, definition or description before mentions in other fields, such as a rationale.

### Added

- Each `verify-evidence` result carries the `cited_lines` its citation records, beside the `lines` where a shifted excerpt now sits.

## 0.19.0

### Added

- `resolve-terms` answers a query no term matches with the reviewed records whose text mentions it, as non-authoritative `mentioned` matches naming the field, instead of an empty list that reads as "not in the Registry".
- `verify-evidence --path` reports each requested path with its citation count and whether the file exists, and accepts `./` and backslash spellings.

## 0.18.2

### Fixed

- `AGENTS.md` no longer says every command takes both roots; it names the commands that take only one. It also says what `boundary-impact`'s `unknown` means, and that `validate` does not check citations against the code. A first-time Agent working from the page made three failed calls because of the old wording.

## 0.18.1

### Fixed

- `AGENTS.md` gives a first-time Agent the vocabulary, a table of common tasks with their commands, and the mistakes that force a redo; `SKILL.md` points to it.

## 0.18.0

### Added

- `verify-evidence` names each citation's record by asset and id, and `--path` limits it to the citations of the given files.

### Changed

- A citation whose excerpt is unchanged but moved within its file, because lines were added above it, is `shifted` with its new lines instead of `stale`, and does not fail `verify-evidence`. `relocate-evidence` with the same `--from` and `--to` re-cites the shifted citations of a file and leaves its current ones alone.

## 0.17.0

### Added

- `recite --asset --id --path --lines [--replace]` re-cites one citation of a record whose code changed in place and prints the update, ready for a Change Package.
- `recover-registry-update` takes `--repo-root` and with it reconciles a pending apply.

### Fixed

- `validate-change-package` refuses a registry update that names an unknown asset, listing the valid ones, and names the asset whose list key was given instead, such as `dependencies` for `dependency-policies`. Such an update used to pass every stage until apply.
- A failed apply no longer leaves its reconciliation marker behind. Reconciling reads the whole audit log: a proposal with no apply event is cleared whatever its status, instead of blocking every later apply.
- `recover-registry-update` says whether there was anything to recover and reports a pending apply that remains, instead of always printing that it recovered.

## 0.16.0

### Added

- `relocate-evidence --from --to` re-cites the evidence under a moved file or directory by its excerpt digest and prints the Change Package updates, the proposals to supersede, the candidates, and the citations it could not place.
- `validate-change-package` validates the Registry as a draft's updates would leave it, prefixing each new error with `once applied:`, and with `--repo-root` requires the updated records' citations to be current.
- `supersedes` may list several proposals, for a package that replaces records reviewed by more than one.
- `record-test-result --cwd <directory>` runs the command in a directory inside the repository and records it.
- A material proposal that changes no Rule, Contract or Aggregate may record its counterfactual check as `not_applicable` with a reason.

### Changed

- A counterfactual check must target one of the design's `proof_obligations`.
- `redraft-proposal` resets the test results and recorded checks, which proved the superseded proposal.

### Fixed

- A Context, Rule or Contract the package itself adds is no longer reported as unknown, and an applied package is no longer checked against ids the Registry retired later.
- A registry update is refused only for errors it introduces, not for ones the Registry already had.

## 0.15.0

### Added

- `upsert-candidate` computes the digests `cite` would give for a citation that has a path and lines but no digest, and keeps a digest it is given.
- `boundary-impact` lists `outside_every_context`, the changed files no Context owns, so a reader knows what its verdict did not cover.

### Fixed

- A citation with no digest is reported as `unhashed`, and `validate` refuses it, instead of being reported as `stale`, which pointed at a file that had not changed.
- `promote-candidate` on a submitted proposal says how to continue: `supersede-proposal`, then `redraft-proposal`.

## 0.14.0

### Added

- `boundary-impact --base <revision>` lists the Contexts a change touches by their `implementation_path`, the registered collaborations between each pair touched together, and the boundary records whose cited evidence changed, with a verdict such as `unregistered-co-change`. It reads no source language. The implementation hygiene and Maintain Skills run it before review.
- A Contract may carry an optional `idempotency` object (`key`, `scope`, `on_replay`, `on_conflict`, optional `expiry`), checked by `validate` and printed by `analyze-boundary`.
- The Registry schema says where a published fact and the policy acting on it are recorded: the fact in the producer's Contract, the policy as a Rule in the consumer's Context.
- The Read Skill lists the tactical design questions itself instead of only linking to them.

## 0.13.0

### Added

- `upsert-candidate` takes `--record-file` more than once and validates and writes the records as one audited update, so terms that name each other in `not_same_as` can be added together. Its audit event lists `record_ids` for a batch and keeps `record_id` for a single record.
- `refine-sources` takes `--add` and `--remove` to change the current selection without repeating it; removing a path that is not selected is refused.

## 0.12.0

### Added

- `resolve-terms` takes `--query` more than once and answers every query in one call; each match names the `query` it answers.

## 0.11.0

### Added

- Vocabulary `synonyms` (names to avoid in favour of the term's `name`) and `not_same_as` (ids of look-alike terms with a different meaning) are defined in the Registry schema and checked by `validate`.
- `resolve-terms` finds a term through its synonyms and marks each match with `match_kind` (`name`, `synonym` or `definition`), name matches first, so a term found only in another term's definition no longer looks resolved.

### Fixed

- `probe` and `readiness` on a root one directory too deep name the likely root instead of reporting that nothing is initialized.

## 0.10.15

### Changed

- The command-line parser registers its commands in six groups; every command, argument, default and the order `--help` lists them in are unchanged. No function in the plugin is exempt from the complexity limits any longer.

## 0.10.14

### Changed

- The source probe checks the policy and the Git fast path in functions of their own; every verdict is unchanged.

## 0.10.13

### Changed

- The secret scanner keeps its counts in one value and admits, reads and skips files in separate steps; its report is unchanged.

### Fixed

- A file that fails while it is being read is reported as an `unreadable` skip, leaving the scan `incomplete`, instead of ending `scan-secrets` with an error.

## 0.10.12

### Changed

- Registry validation checks each asset's header, each record's status, and each kind of context and contract reference in functions of their own; every message is unchanged.

## 0.10.11

### Changed

- Requirement and proposal validation is split into acceptance criteria, risk flags, and proposal fields; every message is unchanged.

## 0.10.10

### Changed

- Implementation design validation checks the classification and presence of a design apart from its fields; every message is unchanged.

## 0.10.9

### Changed

- Citation verification is split into checking the reference's shape, its source file, and its cited lines; every verdict is unchanged.

## 0.10.8

### Changed

- Audit chain verification is split into checking each event, following the chain, and comparing the manifest; every verdict and reason is unchanged.

## 0.10.7

### Changed

- GitHub pull request verification is split into fetching its state and judging the merge, the current approval and the checks; every message is unchanged.

## 0.10.6

### Changed

- Policy validation is split into its enumerated fields, review governance, sources and limits; every message, and governance reporting only its first problem, is unchanged.

## 0.10.5

### Changed

- Approval validation is split into each approval entry and the fields each lifecycle stage requires; every message is unchanged.

## 0.10.4

### Changed

- Applying approved updates is split into finding the finalized proposal, parsing each update, and applying it; every refusal and its message is unchanged.

## 0.10.3

### Changed

- Source map verification is split into its structural checks and the snapshot comparison; every verdict and reason is unchanged.

## 0.10.2

### Changed

- Recovery of an interrupted Registry update is restructured into one handler per journal phase; what it does in each phase is unchanged.

## 0.10.1

### Fixed

- A file-system error, such as `init-change-package --output` naming a file, ends in one `ERROR:` line and exit code 1 instead of a Python traceback.
- `verify-audit` reports `invalid` for a `--registry-root` that holds no Registry, instead of calling an absent audit chain valid.

## 0.10.0

### Changed

- When another proposal's apply changes the Registry, `verify-proposal`, `finalize-proposal` and `apply-approved-updates` rebuild the base Registry from the proposal's observed commit and accept it if the records it touches are unchanged; otherwise it is still stale, and the refusal names what changed. Commit the Registry before `submit-proposal` to benefit.
- `record-test-result` runs its command without a shell, split by POSIX quoting rules on every platform. Pipes, `&&` and `VAR=value` prefixes are no longer available.

### Fixed

- `record-test-result` ran from the command line, and accepts any profile when the policy approves none.

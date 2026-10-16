# Changelog

Notable changes to the Domain Memory plugin, newest first. The format follows Keep a Changelog, and versions follow Semantic Versioning: a minor version adds or changes behavior, a patch version only fixes it. Versions before 0.10.0 are recorded in the repository history only.

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

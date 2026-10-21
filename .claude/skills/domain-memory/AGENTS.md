# Domain Memory

A file-backed Registry of reviewed domain facts, and the commands that read it and change it under review. The next Agent inherits what this one established about the domain, instead of re-deriving it from the code and guessing where it was wrong.

Nothing here is specific to one Agent or one host. The durable state is files in the repository (the facts as Markdown in the Open Knowledge Format, the rest as JSON), the behavior is Markdown plus Python, and the review boundary is Git.

## How it fits together

Everything lives under one Registry root, inside the repository:

```text
<registry-root>/
  registry/                   the facts: one Markdown concept per record, one folder per asset, index.md files
  source-map.json             the repository paths the facts may cite, with a snapshot of each
  domain-memory-policy.json   who may approve a fact, and how that approval is checked
  audit/events.jsonl          a hash-linked log of every write; two clones' writes merge, reconcile-audit joins them
  registry/<asset>/index.md   derived from the concept files; refresh-indexes rebuilds it after a merge
  changes/<package>/          Change Packages, by convention; any directory inside the repository works
```

A fact moves along one of three paths, and only the scripts write:

1. **Read.** Lookups, `validate` and `verify-evidence` change nothing. Reviewed records are the facts you may build on.
2. **Candidate.** `upsert-candidate` adds or revises a candidate at once, with no review. It is handoff material: it never becomes a fact by itself, and it cannot overwrite a reviewed record.
3. **Review.** A Change Package carries `registry_updates`. Start one with `init-change-package` and write the updates, or copy an existing candidate in with `promote-candidate`; a fact need not be a candidate first. Then run, in order:
   1. `submit-proposal` freezes the package against the Registry's current revision.
   2. `record-approval` records each reviewer, and `record-test-result` records each planned test.
   3. `verify-proposal` checks the test results.
   4. `ship-package` checks proof from outside the package, a merged pull request or a signed commit as the policy says, then writes the package. Only then is a record `reviewed`.

Every write takes the same route. It takes a lock, copies the Registry aside, applies the change to the copy, and validates the whole copy. A write that adds a new error is refused. A valid copy is swapped in and the write is appended to the audit chain. An interrupted write is finished or rolled back by `recover-registry-update`.

Four checks keep the facts honest over time:

- Each citation pins the cited lines by hash, so `verify-evidence` can tell whether a fact still stands on the code.
- A proposal goes stale when the records it touches change after it was submitted.
- The source map's snapshots show when the selected sources have moved since a developer confirmed them.
- A Registry whose policy is `local-draft-only` names no outside authority, so it holds candidates only.

## Running the commands

```bash
python3 scripts/registry_tools.py <command> [options]
```

Python 3.10 or later, its standard library, and `git` on the path. Call the interpreter by the name your system gives it: `python3` on most Linux and macOS systems, `python` on Windows, where `python3` is often a store stub. Nothing to install, no environment to prepare, and the script runs from any working directory. What a command prints, and how it refuses, follows the three cases in [script-api.md](references/script-api.md#checks-on-the-code-being-written); read a refusal's reason rather than retrying.

Commands that read or change the Registry take `--registry-root <root>`: the directory that holds `registry/`, `source-map.json` and the policy, not `registry/` itself. Commands that read repository files take `--repo-root <repo>`; the everyday Registry readers accept it unused, so one set of flags fits every command, and an argument a command does not take is refused with that command's own usage. Some take only one of the two: `quality-gates` takes only `--repo-root`, `readiness` needs only `--repo-root`, and the lookups (`lookup`, `resolve-terms`, `get-context`, `get-record`, `dependents`) take only `--registry-root`; `verify-sources` takes `--source-map` in its place. The table below leaves both flags out, and `<command> --help` lists the exact flags.

## Call it from a host as MCP

`mcp-serve` offers the read commands as tools to a host that speaks the Model Context Protocol, so the host calls them without starting the script for each one. Register it as a stdio server, with `python` in place of `python3` on Windows:

```json
{"command": "python3", "args": ["<path>/scripts/registry_tools.py", "mcp-serve", "--registry-root", "<root>", "--repo-root", "<repo>"]}
```

It offers no command that writes; candidates and Change Packages stay on the command line.

## Words you will meet

- **Registry**: one Markdown concept file per record under `<root>/registry/<asset>/`. The asset name is the folder name, such as `contexts`, `rules`, `contracts`, `vocabulary` or `dependency-policies`, and messages name a record as `<asset>/<id>`. Read it through the commands, never its files.
- **Record**: one entry in an asset. Its `status` is `candidate` (proposed, authorizes nothing), `reviewed` (a fact, approved through a Change Package), `deprecated` (a fact no longer in force) or `superseded` (replaced by a newer record).
- **Citation**: a record's evidence, `{path, lines, excerpt_sha256}`, always built by `cite`. `verify-evidence` reports each one with one of these statuses:
  - `current`: the cited lines are unchanged.
  - `shifted`: the excerpt is unchanged but sits on other lines. This does not fail.
  - `stale`: the excerpt changed.
  - `unhashed`: the citation was not built by `cite`.
  - `missing`: the cited file is gone.
  - `invalid`: the citation is malformed, points outside the repository, or cannot be read as cited.
- **Change Package**: the one way a reviewed record changes. It is a directory of four JSON files and a Draft PR description whose proposal moves `draft → submitted → verified → approved → applied`, or ends `superseded`. Once submitted, its content is frozen.

## Common tasks

| You want to | Run |
| --- | --- |
| Know what this repository has | `readiness --repo-root <repo> [--registry-root <root>]`; `extract` means code exists and the Registry holds no record yet. Without `--registry-root` it assesses the one Domain Memory the repository holds and names it; with several it names them and assesses none until you choose |
| Start a Domain Memory | `init-domain-memory`, which records the destination, sources and policy choices a Registry needs |
| Read the model before coding | `resolve-terms --query <word>`, `get-context --id <id>`, `get-record --asset <asset> --id <id>`, and before changing a record `dependents --asset <asset> --id <id>` for what else it may touch; a `mentioned` match is prose in a reviewed record, not a term |
| Review a branch for what it does to the model | the `domain-memory-review-change` capability: `brief --base <branch point> --markdown`, then the verdict table and the attention list decide what to ask for |
| Read one brief before a review or a change | `brief --base <branch point>` or `brief --asset <asset> --id <id>`, `--markdown` for the pull request: verdict, Contexts touched, records to re-read with their dependents, citation verification and an attention list |
| See which boundaries your change touches | `boundary-impact --base <branch point>`; `unknown` means no Context records an `implementation_path`, so it cannot map files to Contexts |
| Catch stale citations before they are committed | `install-pre-commit-hook` once per clone: a commit staging a file whose citations no longer match is refused, and any `boundary-impact` verdict other than `no-cross-context-change` is printed as a note that does not refuse |
| Check the Registry's shape and references | `validate`; it does not check that citations still match the code, which is `verify-evidence` |
| Check the evidence of files you edited | `verify-evidence --path <file>`; `stale` needs `recite` after you check the fact. `shifted` passes every check and needs no Change Package of its own: re-cite it with `relocate-evidence --from <file> --to <file>` in the next package that touches the record; a directory, or `.`, covers everything under it |
| Re-cite after moving code | `relocate-evidence --from <old> --to <new>`; pass the path as the repository holds it; a Context whose `implementation_path` ends `from` gets its new path in the same output |
| Re-cite after editing code in place | `recite --asset <asset> --id <id> --path <file> --lines <a>-<b>`; when the record cites the file more than once, name the old citation with `--replace <file>:<a>-<b>`, and leave out `--lines` if the code kept those lines; add `--onto <file>` holding what an earlier `relocate-evidence` or `recite` printed, and the output is that file with this record re-cited in it, so one record never gets two competing updates |
| Put a re-citation through review | `init-change-package --registry-root <root> --output <dir> --recitation <file printed by recite or relocate-evidence> --requirement-id <id> --proposal-id <id> --proposer <you>` writes the whole package, and this one does need `--registry-root`; then submit it, record its one test result, and review it as usual |
| Rebase a stale proposal | `rebase-proposal --package-root <package>`, when verify, finalize or `ship-package` says the proposal base is stale: it restamps the base and retires the approvals; then `record-approval`, `verify-proposal`, a new signed commit holding the package, and `ship-package --package-root <package> --commit <that commit>` |
| Finish a verified package after its signed commit | `ship-package --package-root <package> --commit <sha>`: attests the signed commit, finalizes the proposal and applies it, in that order, stopping at the first refusal; no git writes |
| Prove rules the code already enforces and package them | `harvest --spec <file> [--through verified]`: counterfactual per rule, candidate, package, preview, and on request submit, test results, approval and verify; stops before the signed commit. After a failed step, run the same spec again to resume, or `discard-harvest --package-root <package> --reason <text>` to undo it, which also undoes a verified harvest that will not be shipped |
| Extract the model from a requirement, code or a change | follow `skills/domain-memory-extract`; `change-coupling --repo-root <repo> --root <code root>` shows which modules change together, and `compare-registry` scores an extraction against a reviewed Registry |
| Propose a new fact | `cite --path <file> --lines <a>-<b> --show`, then `upsert-candidate --asset <asset> --record-file <file>`; give `--record-file` once per record |
| Undo a candidate | `retract-candidate --asset <asset> --id <id> --reason <text>`; a reviewed record is withdrawn by a superseding proposal instead |
| See every Change Package and its state | `list-proposals --changes-root <dir>` |
| Change a reviewed fact | `init-change-package --output <dir> --requirement-id <id> --proposal-id <id> --proposer <you>`, add `registry_updates`, then `validate-change-package --package-root <dir> --as-status <the next status>` before each transition; the lifecycle is in [references/script-api.md](references/script-api.md) |
| Recover after a crash | `recover-registry-update --repo-root <repo>` |

The two re-cite commands print `registry_updates` and `supersedes` for a Change Package. A record that is not reviewed comes back under `candidates`; pass it to `upsert-candidate`.

## Mistakes that cost a redo

- A wrong asset name, such as `dependencies` for `dependency-policies`, is refused by `validate-change-package`. Run it before you submit; after submission, the only fix is to supersede and redraft.
- Replacing or removing a reviewed record needs `supersedes` to list the proposal that reviewed it, which the audit log records when that proposal is applied; `recite` and `relocate-evidence` name it for you. A package that replaces several takes a list.
- A submitted proposal cannot be edited. Use `supersede-proposal --reason`, then `redraft-proposal`. A redraft keeps the design but not the test results, so record them again.
- Cite lines whose text occurs once in the file. A citation whose text repeats elsewhere in the file goes `stale` at the next edit, because nothing can tell the copies apart. `cite` and `recite` print a NOTE when that happens; widen the span to the enclosing block.
- Once a commit is signed or attested, it must reach the remote unchanged. Take in upstream work with a merge (`git pull --no-rebase`), not a rebase: a rebase rewrites the commit without its signature, and an attestation naming it then names a commit that no longer exists.
- Never write a citation by hand. Hand-made line splitting verifies as `stale`, and a missing digest as `unhashed`.
- Errors print `ERROR: <reason>` on stderr and exit non-zero. The reason names the fix, so read it before trying anything else.

## Running the plugin's own tests

To change the plugin itself, read `README.md` in the complete package.

## Before you rely on anything here

Run `readiness --repo-root <repo> [--registry-root <root>]` first; it finds the Domain Memory the repository holds by itself, so give `--registry-root` when there are several or it lives outside the repository. It distinguishes an empty repository, a greenfield design, a brownfield implementation, and a memory whose evidence has died. Its signals route your work; they are not domain facts.

Treat only `reviewed` records as facts. A `candidate` is handoff material and authorizes no implementation.

Not every package is a Context. A Context owns domain rules and the data they decide about. A package that only serves others, such as persistence, transport, a shared loop, or the client of an outside system, is recorded by the repository's own architecture records, not in the Registry. Moving such a package changes no Registry fact while the rules and their data stay with the Context that owns them.

## Choosing what to do

[SKILL.md](SKILL.md) beside this file is where the work is chosen. It routes the seven capabilities to a page under `skills/`. It is plain Markdown, so read it whether or not your host has a concept of Skills.

[references/script-api.md](references/script-api.md) is the command surface and what each command guarantees. [references/host-integration.md](references/host-integration.md) is for wiring this into a host that dispatches work to several Agents.

## When you are the one writing the code

The reviewed facts say what must stay true. These six hold for every change to source, and they are stated here because this page is the one every Agent reads:

1. Run `quality-gates --repo-root <repo>`. The checks it lists must pass, and a check met by a change of form alone is not met: when one forces a change, remove the cause it points at. A flag kept as a keyword argument or a default is a change of form; when it chooses between kinds the domain names, including a kind the handoff says is coming, name that concept in this change. When `standard` is `none`, say so in your report and claim no standard the repository does not enforce.
2. Search for what already exists, and use it or extend it.
3. Take names from the reviewed terms. A business number, such as a threshold, a rate, or a limit, gets the domain's name and one home, and every place it appears uses that name.
4. Every rule you add or move ends with a test in the repository that fails when the rule is broken. Prove it with `counterfactual`, once for each rule: `survived` means no test protects that rule, so write the test and run it again. A comparison you run once and discard does not count. Report each rule that survived before you wrote its test.
5. An external system goes behind a Port named in the domain's terms and declared where a decision can reach it without loading the provider. Address, credentials, and status codes stay in the Adapter. A failure the domain survives is handed to the caller or recorded, never dropped.
6. A mechanical change gets the change, the tests, and a short report.

The reasoning, the evidence each claim needs, and the cases where these do not apply are in [references/code-expression.md](references/code-expression.md) and [references/ports-and-adapters.md](references/ports-and-adapters.md).

## What it will not do

It will not promote a record to reviewed without an approval path that can be checked from outside the package, and it will not let a command claim a result it did not compute. Both refusals are the point: a fact nobody can check is worse than a gap you can see.

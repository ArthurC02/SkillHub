# Domain Memory

A file-backed Registry of reviewed domain facts, and the commands that read it and change it under review. The next Agent inherits what this one established about the domain, instead of re-deriving it from the code and guessing where it was wrong.

Read this page once. Then [SKILL.md](SKILL.md) chooses the capability for the work at hand, [references/script-api.md](references/script-api.md) holds every command with what it guarantees and how it refuses, and [references/host-integration.md](references/host-integration.md) is for a host that dispatches work to several Agents. To change the plugin itself, read `README.md`.

## Two rules that hold whoever is reading

- Run `readiness --repo-root <repo>` before anything else. It is read-only, finds the Domain Memory the repository holds by itself (`--registry-root` chooses among several, or names one outside the repository), tells an empty repository from a greenfield design, a brownfield implementation and a memory whose evidence has died, and names the capability to start with. Its signals route your work; they are not domain facts.
- Treat only `reviewed` records as facts. A `candidate` is handoff material and authorizes no implementation.

## What is on disk

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

Only the scripts write. Every write is applied to a copy of the Registry, validated whole, swapped in and appended to the audit chain, as [the write protocol](references/reliability-architecture.md#the-write-protocol) describes; `recover-registry-update` finishes or rolls back an interrupted one.

## How a fact changes

A record moves along one of three paths:

1. **Read.** Lookups, `validate` and `verify-evidence` change nothing.
2. **Candidate.** `upsert-candidate` adds or revises a candidate at once, with no review. It never becomes a fact by itself, and it cannot overwrite a reviewed record.
3. **Review.** A Change Package carries `registry_updates`, started by `init-change-package` or by `promote-candidate`; a fact need not be a candidate first. `submit-proposal` freezes it against the Registry's current revision, `record-approval` and `record-test-result` record each reviewer and each planned test, `verify-proposal` checks the test results, and `ship-package` checks proof from outside the package, a commit signed by a key the policy authorizes, then writes it. Only then is a record `reviewed`. [Proposal lifecycle](references/proposal-lifecycle.md) has each state. Until a package is applied, `discard-harvest --package-root <package> --reason <text>` withdraws it, retracting the candidates no other package names and leaving the audit log as it was.

## Running the commands

```bash
python3 scripts/registry_tools.py <command> [options]
```

The same commands also run as a compiled Rust executable, which needs no Python and answers faster; [domain-memory-cli](skills/domain-memory-cli/SKILL.md) names the committed executable for Linux and Windows, how to build one elsewhere, how to call it and which commands it answers yet.

Python 3.10 or later, its standard library, and `git` on the path. Call the interpreter by the name your system gives it: `python3` on most Linux and macOS systems, `python` on Windows, where `python3` is often a store stub. Nothing to install, and the script runs from any working directory. What a command prints and how it refuses: [output and refusals](references/script-api.md#output-and-refusals).

Commands that read or change the Registry take `--registry-root <root>`: the directory that holds `registry/`, `source-map.json` and the policy, not `registry/` itself. Commands that read repository files take `--repo-root <repo>`. The table below leaves both flags out; `<command> --help` lists the exact flags, and an argument a command does not take is refused with that command's usage.

A host that speaks the Model Context Protocol gets the read commands as tools from [`mcp-serve`](references/script-api.md#mcp-serve), which offers no command that writes.

## Words you will meet

- **Registry**: one Markdown concept file per record under `<root>/registry/<asset>/`. The asset name is the folder name, such as `contexts`, `rules`, `contracts`, `vocabulary` or `dependency-policies`, and messages name a record as `<asset>/<id>`. Read it through the commands, never its files.
- **Record**: one entry in an asset. Its `status` is `candidate` (proposed, authorizes nothing), `reviewed` (a fact, approved through a Change Package), `deprecated` (a fact no longer in force) or `superseded` (replaced by a newer record).
- **Context**: a package that owns domain rules and the data they decide about. A package that only serves others, such as persistence, transport, a shared loop, or the client of an outside system, is recorded by the repository's own architecture records, not here; moving it changes no Registry fact while the rules and their data stay with the Context that owns them.
- **Citation**: a record's evidence, `{path, lines, excerpt_sha256}`, always built by `cite`. `verify-evidence` reports each one as `current` (the cited lines are unchanged), `shifted` (the excerpt sits on other lines; this does not fail), `stale` (the excerpt changed), `unhashed` (not built by `cite`), `missing` (the file is gone) or `invalid` (malformed, outside the repository, or unreadable as cited).
- **Change Package**: the one way a reviewed record changes. It is a directory of four JSON files and a Draft PR description whose proposal moves `draft → submitted → verified → approved → applied`, or ends `superseded`. Once submitted, its content is frozen.

## Common tasks

Each row names the command and the flags that matter; [script-api.md](references/script-api.md) has every option, what the command guarantees and how it refuses.

| You want to | Run |
| --- | --- |
| Know what this repository has | `readiness --repo-root <repo> [--registry-root <root>]` |
| Start a Domain Memory | `discover-sources --repo-root <repo>`, then `init-domain-memory` with the destination, sources and policy a developer chose |
| Read the model before coding | `resolve-terms --query <word>`, `get-context --id <id>`, `get-record --asset <asset> --id <id>`; `dependents --asset <asset> --id <id>` before changing a record |
| Find the table a request is about | `describe-table --repo-root <repo> --migrations <dir>` lists every table with its meaning and gap count |
| Read or write a table | `describe-table --repo-root <repo> --migrations <dir> --table <name> [--queries <dir>]` |
| Finish a migration | `describe-table --repo-root <repo> --migrations <dir> --table <name> --require-complete`, or `--base <ref>` in place of `--table`; exit 0 means done |
| Review a branch against the model | `brief --base <branch point> --markdown` |
| See which boundaries a change touches | `boundary-impact --base <branch point>` |
| Check the Registry | `validate`; `verify-evidence` for whether citations still match the code; `coverage` for the Contexts that still lack rules |
| Check files you edited | `verify-evidence --path <file>` |
| Re-cite after moving or editing code | `relocate-evidence --from <old> --to <new>`; `recite --asset <asset> --id <id> --path <file> --lines <a>-<b>` |
| Put a re-citation through review | `init-change-package --output <dir> --recitation <file> --requirement-id <id> --proposal-id <id> --proposer <you>` |
| Prove rules the code already enforces | `harvest --spec <file> [--through verified]`; `discard-harvest --package-root <package> --reason <text>` undoes it |
| Prove that a test protects a rule | `counterfactual --repo-root <repo> --file <path> --find <text> --replace <text> --test-command <command>` |
| Catch stale citations before they are committed | `install-pre-commit-hook`, once per clone |
| Propose a new fact | `cite --path <file> --lines <a>-<b> --show`, then `upsert-candidate --asset <asset> --record-file <file>` |
| Undo a candidate | `retract-candidate --asset <asset> --id <id> --reason <text>` |
| Change a reviewed fact | `init-change-package --output <dir> --requirement-id <id> --proposal-id <id> --proposer <you>`, then `validate-change-package --package-root <dir> --as-status <next status>` before each transition |
| Finish a verified package | `ship-package --package-root <package> --commit <sha>`; `rebase-proposal --package-root <package>` first when it says the base is stale |
| Recover after a crash | `recover-registry-update --repo-root <repo>` |

## Mistakes that cost a redo

- A wrong asset name, such as `dependencies` for `dependency-policies`, is refused by `validate-change-package`. Run it before you submit; after submission, the only fix is to supersede and redraft.
- Replacing or removing a reviewed record needs `supersedes` to list the proposal that reviewed it, which the audit log records when that proposal is applied; `recite` and `relocate-evidence` name it for you. A package that replaces several takes a list.
- A submitted proposal cannot be edited. Use `supersede-proposal --reason`, then `redraft-proposal`. A redraft keeps the design but not the test results, so record them again.
- Cite lines whose text occurs once in the file. A citation whose text repeats elsewhere in the file goes `stale` at the next edit, because nothing can tell the copies apart. `cite` and `recite` print a NOTE when that happens; widen the span to the enclosing block.
- Once a commit is signed or attested, it must reach the remote unchanged. Take in upstream work with a merge (`git pull --no-rebase`), not a rebase: a rebase rewrites the commit without its signature, and an attestation naming it then names a commit that no longer exists.
- Never write a citation by hand. Hand-made line splitting verifies as `stale`, and a missing digest as `unhashed`.
- Errors print `ERROR: <reason>` on stderr and exit non-zero. The reason names the fix, so read it before trying anything else.

## When you are the one writing the code

The reviewed facts say what must stay true. These seven hold for every change to source, and they are stated here because this page is the one every Agent reads:

1. Run `quality-gates --repo-root <repo>`. The checks it lists must pass, and a check met by a change of form alone is not met: when one forces a change, remove the cause it points at. A flag kept as a keyword argument or a default is a change of form; when it chooses between kinds the domain names, including a kind the handoff says is coming, name that concept in this change. When `standard` is `none`, say so in your report and claim no standard the repository does not enforce.
2. Search for what already exists, and use it or extend it.
3. Take names from the reviewed terms. A business number, such as a threshold, a rate, or a limit, gets the domain's name and one home, and every place it appears uses that name.
4. Every rule you add or move ends with a test in the repository that fails when the rule is broken. Prove it with `counterfactual`, once for each rule: `survived` means no test protects that rule, so write the test and run it again. A comparison you run once and discard does not count. Report each rule that survived before you wrote its test.
5. An external system goes behind a Port named in the domain's terms and declared where a decision can reach it without loading the provider. Address, credentials, and status codes stay in the Adapter. A failure the domain survives is handed to the caller or recorded, never dropped.
6. A mechanical change gets the change, the tests, and a short report.
7. A migration that creates or alters a table is done when `describe-table --repo-root <repo> --migrations <dir> --table <name> --require-complete` exits 0, or `--base <ref>` in place of `--table` when older tables carry gaps of their own. `gaps` lists what is missing, and [describe-table](references/script-api.md#describe-table) defines each gap: a table or column without a `COMMENT ON`, a status-like column without a value list, a value list that leaves NULL undecided, a listed value the comment does not explain, a foreign key without `ON DELETE`, and DDL built at run time.

The reasoning, the evidence each claim needs, and the cases where these do not apply are in [references/code-expression.md](references/code-expression.md) and [references/ports-and-adapters.md](references/ports-and-adapters.md).

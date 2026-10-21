# Domain Memory

A file-backed Registry of reviewed domain facts, and the commands that read it and change it under review. The next Agent inherits what this one established about the domain, instead of re-deriving it from the code and guessing where it was wrong.

Nothing here is specific to one Agent or one host: the state is files in the repository, the behavior is Markdown plus Python, and the review boundary is Git. Read this page once. Then [SKILL.md](SKILL.md) chooses the capability for the work at hand, [references/script-api.md](references/script-api.md) holds every command with what it guarantees and how it refuses, and [references/host-integration.md](references/host-integration.md) wires this into a host that dispatches work to several Agents. To change the plugin itself, read `README.md`.

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

Only the scripts write, and every write takes the same route: a lock, a copy of the Registry, the change applied to the copy, the whole copy validated. A write that adds a new error is refused; a valid copy is swapped in and the write is appended to the audit chain. An interrupted write is finished or rolled back by `recover-registry-update`. The trust boundaries behind this are in [reliability architecture](references/reliability-architecture.md).

## How a fact changes

A record moves along one of three paths:

1. **Read.** Lookups, `validate` and `verify-evidence` change nothing.
2. **Candidate.** `upsert-candidate` adds or revises a candidate at once, with no review. It never becomes a fact by itself, and it cannot overwrite a reviewed record.
3. **Review.** A Change Package carries `registry_updates`. Start one with `init-change-package` and write the updates, or copy an existing candidate in with `promote-candidate`; a fact need not be a candidate first. Then, in order: `submit-proposal` freezes the package against the Registry's current revision; `record-approval` records each reviewer and `record-test-result` each planned test; `verify-proposal` checks the test results; `ship-package` checks proof from outside the package, a merged pull request or a signed commit as the policy says, then writes the package. Only then is a record `reviewed`.

## Running the commands

```bash
python3 scripts/registry_tools.py <command> [options]
```

Python 3.10 or later, its standard library, and `git` on the path. Call the interpreter by the name your system gives it: `python3` on most Linux and macOS systems, `python` on Windows, where `python3` is often a store stub. Nothing to install, and the script runs from any working directory. What a command prints and how it refuses: [output and refusals](references/script-api.md#output-and-refusals).

Commands that read or change the Registry take `--registry-root <root>`: the directory that holds `registry/`, `source-map.json` and the policy, not `registry/` itself. Commands that read repository files take `--repo-root <repo>`; the everyday Registry readers accept it unused, so one set of flags fits every command. The table below leaves both flags out; `<command> --help` lists the exact flags, and an argument a command does not take is refused with that command's usage.

A host that speaks the Model Context Protocol gets the read commands as tools from `mcp-serve`, registered as a stdio server, with `python` in place of `python3` on Windows:

```json
{"command": "python3", "args": ["<path>/scripts/registry_tools.py", "mcp-serve", "--registry-root", "<root>", "--repo-root", "<repo>"]}
```

It offers no command that writes; candidates and Change Packages stay on the command line.

## Words you will meet

- **Registry**: one Markdown concept file per record under `<root>/registry/<asset>/`. The asset name is the folder name, such as `contexts`, `rules`, `contracts`, `vocabulary` or `dependency-policies`, and messages name a record as `<asset>/<id>`. Read it through the commands, never its files.
- **Record**: one entry in an asset. Its `status` is `candidate` (proposed, authorizes nothing), `reviewed` (a fact, approved through a Change Package), `deprecated` (a fact no longer in force) or `superseded` (replaced by a newer record).
- **Context**: a package that owns domain rules and the data they decide about. A package that only serves others, such as persistence, transport, a shared loop, or the client of an outside system, is recorded by the repository's own architecture records, not here; moving it changes no Registry fact while the rules and their data stay with the Context that owns them.
- **Citation**: a record's evidence, `{path, lines, excerpt_sha256}`, always built by `cite`. `verify-evidence` reports each one as `current` (the cited lines are unchanged), `shifted` (the excerpt sits on other lines; this does not fail), `stale` (the excerpt changed), `unhashed` (not built by `cite`), `missing` (the file is gone) or `invalid` (malformed, outside the repository, or unreadable as cited).
- **Change Package**: the one way a reviewed record changes. It is a directory of four JSON files and a Draft PR description whose proposal moves `draft → submitted → verified → approved → applied`, or ends `superseded`. Once submitted, its content is frozen.

## Common tasks

| You want to | Run |
| --- | --- |
| Know what this repository has | `readiness --repo-root <repo> [--registry-root <root>]`; `extract` means code exists and the Registry holds no record yet |
| Start a Domain Memory | `init-domain-memory`, which records the destination, sources and policy choices a Registry needs |
| Read the model before coding | `resolve-terms --query <word>`, `get-context --id <id>`, `get-record --asset <asset> --id <id>`; before changing a record, `dependents --asset <asset> --id <id>` lists what else it may touch |
| Review a branch for what it does to the model | `brief --base <branch point> --markdown`, or `brief --asset <asset> --id <id>` for one record; [domain-memory-review-change](skills/domain-memory-review-change/SKILL.md) turns the verdict and the attention list into what to ask for |
| See which boundaries your change touches | `boundary-impact --base <branch point>`; `unknown` means no Context records an `implementation_path` |
| Catch stale citations before they are committed | `install-pre-commit-hook`, once per clone |
| Check the Registry's shape and references | `validate`; whether citations still match the code is `verify-evidence` |
| Check the evidence of files you edited | `verify-evidence --path <file>`; `stale` needs `recite` after you check the fact, `shifted` passes every check and is re-cited with `relocate-evidence --from <file> --to <file>` in the next package that touches the record |
| Re-cite after moving code | `relocate-evidence --from <old> --to <new>`, with the path as the repository holds it |
| Re-cite after editing code in place | `recite --asset <asset> --id <id> --path <file> --lines <a>-<b>`; `--onto <file>` chains it after what an earlier `relocate-evidence` or `recite` printed, so one record never gets two competing updates |
| Put a re-citation through review | `init-change-package --registry-root <root> --output <dir> --recitation <file printed by recite or relocate-evidence> --requirement-id <id> --proposal-id <id> --proposer <you>` writes the whole package; then submit it, record its one test result, and review it as usual |
| Rebase a stale proposal | `rebase-proposal --package-root <package>` when verify, finalize or `ship-package` says the base is stale; then `record-approval`, `verify-proposal`, a new signed commit holding the package, and `ship-package --package-root <package> --commit <that commit>` |
| Finish a verified package after its signed commit | `ship-package --package-root <package> --commit <sha>`: attests, finalizes and applies, stopping at the first refusal; no git writes |
| Prove rules the code already enforces and package them | `harvest --spec <file> [--through verified]`, which stops before the signed commit; run the same spec again to resume after a failed step, or `discard-harvest --package-root <package> --reason <text>` to undo it |
| Extract the model from a requirement, code or a change | [domain-memory-extract](skills/domain-memory-extract/SKILL.md); `change-coupling --repo-root <repo> --root <code root>` shows which modules change together, and `compare-registry` scores an extraction against a reviewed Registry |
| Propose a new fact | `cite --path <file> --lines <a>-<b> --show`, then `upsert-candidate --asset <asset> --record-file <file>`, once per record |
| Undo a candidate | `retract-candidate --asset <asset> --id <id> --reason <text>`; a reviewed record is withdrawn by a superseding proposal instead |
| See every Change Package and its state | `list-proposals --changes-root <dir>` |
| Change a reviewed fact | `init-change-package --output <dir> --requirement-id <id> --proposal-id <id> --proposer <you>`, add `registry_updates`, then `validate-change-package --package-root <dir> --as-status <the next status>` before each transition; [proposal lifecycle](references/proposal-lifecycle.md) has the states |
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

## When you are the one writing the code

The reviewed facts say what must stay true. These six hold for every change to source, and they are stated here because this page is the one every Agent reads:

1. Run `quality-gates --repo-root <repo>`. The checks it lists must pass, and a check met by a change of form alone is not met: when one forces a change, remove the cause it points at. A flag kept as a keyword argument or a default is a change of form; when it chooses between kinds the domain names, including a kind the handoff says is coming, name that concept in this change. When `standard` is `none`, say so in your report and claim no standard the repository does not enforce.
2. Search for what already exists, and use it or extend it.
3. Take names from the reviewed terms. A business number, such as a threshold, a rate, or a limit, gets the domain's name and one home, and every place it appears uses that name.
4. Every rule you add or move ends with a test in the repository that fails when the rule is broken. Prove it with `counterfactual`, once for each rule: `survived` means no test protects that rule, so write the test and run it again. A comparison you run once and discard does not count. Report each rule that survived before you wrote its test.
5. An external system goes behind a Port named in the domain's terms and declared where a decision can reach it without loading the provider. Address, credentials, and status codes stay in the Adapter. A failure the domain survives is handed to the caller or recorded, never dropped.
6. A mechanical change gets the change, the tests, and a short report.

The reasoning, the evidence each claim needs, and the cases where these do not apply are in [references/code-expression.md](references/code-expression.md) and [references/ports-and-adapters.md](references/ports-and-adapters.md).

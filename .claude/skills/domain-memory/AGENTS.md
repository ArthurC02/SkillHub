# Domain Memory

A file-backed Registry of reviewed domain facts, and the commands that read it and change it under review. The next Agent inherits what this one established about the domain, instead of re-deriving it from the code and guessing where it was wrong.

Nothing here is specific to one Agent or one host. The durable state is JSON files in the repository, the behavior is Markdown plus Python, and the review boundary is Git.

## Running the commands

```bash
python3 scripts/registry_tools.py <command> [options]
```

Python 3.10 or later, its standard library, and `git` on the path. Nothing to install, no environment to prepare, and the script runs from any working directory. Output is JSON except for success and error lines. A non-zero exit is a refusal that names its reason; read the reason rather than retrying.

Every command below takes `--registry-root <root>`, the directory that holds `registry/`, `source-map.json` and the policy, not `registry/` itself. Commands that read files in the repository also take `--repo-root <repo>`.

## Words you will meet

- **Registry**: one JSON file per asset under `<root>/registry/`. The asset name is the file name without `.json`, such as `contexts`, `rules`, `contracts`, `vocabulary` or `dependency-policies`. It is never the list key inside the file.
- **Record**: one entry in an asset. Its `status` is `candidate` (proposed, authorizes nothing) or `reviewed` (a fact, approved through a Change Package).
- **Citation**: a record's evidence, `{path, lines, excerpt_sha256}`, always built by `cite`. `verify-evidence` reports each one with one of these statuses:
  - `current`: the cited lines are unchanged.
  - `shifted`: the excerpt is unchanged but sits on other lines. This does not fail.
  - `stale`: the excerpt changed.
  - `unhashed`: the citation was not built by `cite`.
  - `missing`: the cited file is gone.
- **Change Package**: the one way a reviewed record changes. It is a directory of four JSON files whose proposal moves `draft → submitted → verified → approved → applied`, or ends `superseded`. Once submitted, its content is frozen.

## Common tasks

| You want to | Run |
| --- | --- |
| Know what this repository has | `readiness --repo-root <repo>` |
| Read the model before coding | `resolve-terms --query <word>`, `get-context --id <id>`, `get-record --asset <asset> --id <id>` |
| See which boundaries your change touches | `boundary-impact --base <branch point>` |
| Check the evidence of files you edited | `verify-evidence --path <file>`; `stale` needs `recite`, `shifted` needs `relocate-evidence --from <file> --to <file>` |
| Re-cite after moving code | `relocate-evidence --from <old> --to <new>` |
| Re-cite after editing code in place | `recite --asset <asset> --id <id> --path <file> --lines <a>-<b>` |
| Propose a new fact | `cite`, then `upsert-candidate --asset <asset> --record-file <file>`; give `--record-file` once per record |
| Change a reviewed fact | `init-change-package`, add `registry_updates`, then `validate-change-package --as-status submitted --repo-root <repo>` before every transition; the lifecycle is in [references/script-api.md](references/script-api.md) |
| Recover after a crash | `recover-registry-update --repo-root <repo>` |

The two re-cite commands print `registry_updates` and `supersedes` for a Change Package. A record that is not reviewed comes back under `candidates`; pass it to `upsert-candidate`.

## Mistakes that cost a redo

- A wrong asset name, such as `dependencies` for `dependency-policies`, is refused by `validate-change-package`. Run it before you submit; after submission, the only fix is to supersede and redraft.
- Replacing or removing a reviewed record needs `supersedes` to list the proposal that reviewed it, which is `review.proposal_id` on the record. A package that replaces several takes a list.
- A submitted proposal cannot be edited. Use `supersede-proposal --reason`, then `redraft-proposal`. A redraft keeps the design but not the test results, so record them again.
- Never write a citation by hand. Hand-made line splitting verifies as `stale`, and a missing digest as `unhashed`.
- Errors print `ERROR: <reason>` on stdout and exit non-zero. The reason names the fix, so read it before trying anything else.

## Before you rely on anything here

Run `readiness --repo-root <repo>` first. It distinguishes an empty repository, a greenfield design, a brownfield implementation, and a memory whose evidence has died. Its signals route your work; they are not domain facts.

Treat only `reviewed` records as facts. A `candidate` is handoff material and authorizes no implementation. `reviewed-empty` means there is no model to depend on yet, so go back to the decisions, contracts, code, and tests.

Not every package is a Context. A Context owns domain rules and the data they decide about. A package that only serves others, such as persistence, transport, a shared loop, or the client of an outside system, is recorded by the repository's own architecture records, not in the Registry. Moving such a package changes no Registry fact while the rules and their data stay with the Context that owns them.

## Choosing what to do

[SKILL.md](SKILL.md) beside this file is where the work is chosen. In the complete package it routes the four lifecycle capabilities and the hygiene check to a page under `skills/`; in a standalone bundle it is the one capability the bundle carries. It is plain Markdown, so read it whether or not your host has a concept of Skills.

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

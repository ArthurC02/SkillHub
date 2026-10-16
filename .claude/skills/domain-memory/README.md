# Domain Memory — developing the plugin

Agents start at `AGENTS.md`; this page is for people changing the plugin itself.

Requirements: Python 3.10 or later, the standard library, and `git`. The suite runs on Linux and Windows.

Run from this directory:

```sh
python -m unittest discover -s scripts -p "test_*.py"
ruff check .
```

The suite takes several minutes. While working on one area, run its file from `scripts/`, for example `python -m unittest test_evidence`. Each file covers the modules it is named after: `test_registry` (registry, common, contracts), `test_evidence` (evidence, relocate), `test_sources` (sources, readiness, security, impact), `test_changes` (changes, counterfactual), `test_review` (hitl, attestations, signing, git_hooks), `test_updates` (updates, transaction, audit, revision), `test_policy`, `test_cli` (the command line end to end) and `test_implementation_checks` (gates). Shared fixtures live in `test_support.py`. Run the whole suite before shipping.

## How the code is laid out

`scripts/registry_tools.py` is the only entry point. It calls `cli.main`, which builds the parser in `cli_parser.py` and dispatches to one `handle_*` function in `cli_commands.py`. A handler turns arguments into calls and prints the result; the rules belong in the modules below, all in `scripts/domain_registry/`:

| Module | Owns |
| --- | --- |
| `common.py` | Asset names and formats, bounded JSON loading, the atomic `write_json`, the writer lock, and the shared rules: `record_status`, `utc_now` |
| `registry.py` | Initializing and validating a Registry, lookups, terms, coverage, boundary analysis |
| `store.py` | Reading and writing Registry documents, whichever layout holds them, and reading them at a commit |
| `okf.py` | One record as one Open Knowledge Format concept document, and the YAML subset its frontmatter uses |
| `compare.py` | Scoring an extracted Registry against a reviewed one |
| `transaction.py` | `mutate_registry`, the one write route: lock, stage a copy, validate, swap, audit, recover |
| `audit.py` | The hash-chained `audit/events.jsonl` and its manifest |
| `evidence.py` | Building a citation (`cite`) and judging one (`current`, `shifted`, `stale` and the rest) |
| `relocate.py` | Re-citing moved or edited evidence |
| `recitation_package.py` | Writing a Change Package for a re-citation |
| `sources.py` | Walking the repository, the source map and its snapshots, the source policy |
| `readiness.py`, `gates.py`, `impact.py`, `security.py` | Read-only reports: readiness, quality gates, boundary impact, secret scan |
| `policy.py` | The policy file: defaults, validation, amendments |
| `changes.py` | Validating a Change Package at each lifecycle stage |
| `hitl.py` | Moving a proposal through submit, approve, verify and finalize |
| `updates.py` | Candidates, and applying an approved package |
| `revision.py` | The Registry digest a proposal is pinned to, and when it counts as stale |
| `attestations.py`, `signing.py`, `git_hooks.py` | Outside proof: GitHub pull requests, signed commits, the pre-push hook |
| `counterfactual.py`, `contracts.py` | Breaking one line to prove a test protects it; schema checks |

Three rules keep this shape. Once a Registry exists, its records change only through `mutate_registry`; every Registry read and write goes through `store.py`, so no other module knows the file layout. Every JSON document the plugin builds is written through `write_json`. A rule that two modules need lives once, in the lower one, and the other imports it.

`ruff.toml` pins every finding that predates the lint gate with a line-level `noqa`. A new finding fails, and so does a `noqa` that no longer suppresses anything: simplify a pinned function and its marker has to go in the same change.

Any change to what ships — everything except `evals/` and `scripts/test_*.py` — raises `version` in `.claude-plugin/plugin.json`, so installed copies see it, and adds an entry to `CHANGELOG.md`.

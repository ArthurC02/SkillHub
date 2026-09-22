# Domain Memory

A file-backed Registry of reviewed domain facts, and the commands that read it and change it under review. It exists so the next Agent inherits what this one established about the domain, instead of re-deriving it from the code and guessing where it was wrong.

Nothing here is specific to one Agent or one host. The durable state is JSON files in the repository, the behavior is Markdown plus Python, and the review boundary is Git.

## Running the commands

```bash
python3 scripts/registry_tools.py <command> [options]
```

Python 3.10 or later, its standard library, and `git` on the path. Nothing to install, no environment to prepare, and the script runs from any working directory. Output is JSON except for success and error lines. A non-zero exit is a refusal that names its reason; read the reason rather than retrying.

## Before you rely on anything here

Run `readiness --repo-root <repo>` first. It distinguishes an empty repository, a greenfield design, a brownfield implementation, and a memory whose evidence has died. Its signals route your work; they are not domain facts.

Treat only `reviewed` records as facts. A `candidate` is handoff material and authorizes no implementation. `reviewed-empty` means there is no model to depend on yet, so go back to the decisions, contracts, code, and tests.

## Choosing what to do

[SKILL.md](SKILL.md) beside this file is where the work is chosen. In the complete package it routes the four lifecycle capabilities and the hygiene check to a page under `skills/`; in a standalone bundle it is the one capability the bundle carries. Either way it is plain Markdown, so read it whether or not your host has a concept of Skills.

[references/script-api.md](references/script-api.md) is the command surface and what each command guarantees. [references/host-integration.md](references/host-integration.md) is for wiring this into a host that dispatches work to several Agents.

## What it will not do

It will not promote a record to reviewed without an approval path that can be checked from outside the package, and it will not let a command claim a result it did not compute. Both refusals are the point: a fact nobody can check is worse than a gap you can see.

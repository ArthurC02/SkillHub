# Packaging modes

Use the Plugin layout when the host supports a Plugin containing multiple Skills:

```text
domain-memory/
├── AGENTS.md
├── .claude-plugin/plugin.json
├── skills/domain-memory-extract/SKILL.md
├── skills/domain-memory-read/SKILL.md
├── skills/domain-memory-design/SKILL.md
├── skills/domain-memory-maintain/SKILL.md
├── skills/domain-memory-review/SKILL.md
├── skills/domain-memory-review-change/SKILL.md
└── skills/domain-memory-implementation-hygiene/SKILL.md
```

The Plugin is the complete Domain Memory capability. Its manifest points to `./skills/`, and the seven Skill directories share the references, templates, and scripts at the Plugin root. A plugin install exposes exactly those seven Skills, not the Plugin-root `SKILL.md`, so the router page and its readiness-first instruction never reach the host. [Host integration](host-integration.md) says what a host must do to keep that step.

The manifest carries packaging identity only: `name`, `version`, `description`, `author`, `keywords`, and the component pointers. Presentation and catalogue fields belong to the host or the marketplace entry, not here; a host ignores unknown fields, so anything it does not define is silent drift rather than configuration.

The scripts need Python 3.10 or later, its standard library, and `git`, and nothing else, except that `init-signing-key` alone also needs OpenSSH's `ssh-keygen`. A host that cannot offer both runs the Skills without the controlled write boundary, which means no promotion and no audit chain, rather than partially.

Only hosts that define a manifest read it, so the Plugin root also carries `AGENTS.md`: the entry point for an Agent that arrives with no manifest support and no concept of Skills. It states the two rules that hold whoever is reading, `readiness` first and only `reviewed` records are facts, then what is on disk, how a fact changes, the runtime and command form, the vocabulary, the common tasks, the mistakes that force a redo and the seven code rules, and sends the reader to the router to choose a capability.

The Plugin root also holds `evals/`. Each directory under it states its scenarios in `scenarios.json`: a small project, the handoff and task an Agent receives, the expectations a reader judges, and a `check.py` that computes the ones a machine can. Give each scenario to an Agent with the Plugin as released and again with the Plugin as changed, on more than one model tier, and compare. A change to a Skill or a reference that moves no scenario has not been shown to help.

Before release, validate the Plugin manifest and run the scripts' tests. A valid manifest proves packaging shape; it does not prove that a host invokes every lifecycle capability.

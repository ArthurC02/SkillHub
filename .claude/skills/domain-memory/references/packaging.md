# Packaging modes

Use the Plugin layout when the host supports a Plugin containing multiple Skills:

```text
domain-memory/
├── AGENTS.md
├── .claude-plugin/plugin.json
├── skills/domain-memory-read/SKILL.md
├── skills/domain-memory-design/SKILL.md
├── skills/domain-memory-maintain/SKILL.md
├── skills/domain-memory-review/SKILL.md
└── skills/domain-memory-implementation-hygiene/SKILL.md
```

The Plugin is the complete Domain Memory capability. Its manifest points to `./skills/`, and the five Skill directories share the references, templates, and scripts at the Plugin root. A plugin install exposes exactly those five Skills; it does not expose the Plugin-root `SKILL.md` as a Skill, so the router page and its readiness-first instruction are not part of what an installed plugin surfaces to a host. See [host integration](host-integration.md) for what a host must do to keep that step.

The manifest carries packaging identity only: `name`, `version`, `description`, `author`, `keywords`, and the component pointers. Presentation and catalogue fields belong to the host or the marketplace entry, not here; a host ignores unknown fields, so anything it does not define is silent drift rather than configuration.

The scripts need Python 3.10 or later and nothing outside its standard library, and they shell out to `git` alone. A host that cannot offer both runs the Skills without the controlled write boundary, which means no promotion and no audit chain, rather than partially.

A manifest is read by the hosts that define one, and the Plugin root therefore also carries `AGENTS.md`: the entry point for an Agent that arrives in the directory with no manifest support and no concept of Skills. It states the runtime, the command form, and the two rules that hold whoever is reading — `readiness` first, and only `reviewed` records are facts — then sends the reader to the router. Keep it that shape. Restating the routing there would put the same list in two files that drift apart, and restating the command surface would duplicate `script-api.md`.

When a host accepts only one standalone Skill, build a self-contained bundle first:

```bash
python scripts/build_standalone.py \
  --plugin-root . \
  --skill domain-memory-read \
  --output ./dist/domain-memory-read
```

Upload the resulting directory. Do not upload a directory under `skills/` directly: its references and templates live at the Plugin root. Do not upload the Plugin root as if it were a standalone replacement: its root `SKILL.md` is a router and the host may not discover nested Skills automatically.

Standalone bundles keep the same file-backed Registry format and include the shared resources required by the selected capability. Change Packages use explicit `domain-*/v1` format fields so an older package cannot silently pass as current. They may omit unrelated capabilities, but must not claim that one capability performed the complete Read → Design → Maintain → Review cycle. Implementation hygiene is a companion check, not a substitute for that lifecycle.

Before release, validate the Plugin manifest, each standalone bundle, and the selected bundle's scripts. A valid manifest proves packaging shape; it does not prove that a host invokes every lifecycle capability.

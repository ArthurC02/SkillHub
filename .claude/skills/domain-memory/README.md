# Domain Memory — developing the plugin

Agents start at `AGENTS.md`; this page is for people changing the plugin itself.

Requirements: Python 3.10 or later, the standard library, and `git`. The suite runs on Linux and Windows.

Run from this directory:

```sh
python -m unittest discover -s scripts -p "test_*.py"
ruff check .
```

`ruff.toml` pins every finding that predates the lint gate with a line-level `noqa`. A new finding fails, and so does a `noqa` that no longer suppresses anything: simplify a pinned function and its marker has to go in the same change.

Any change to what ships — everything except `evals/` and `scripts/test_*.py` — raises `version` in `.claude-plugin/plugin.json`, so installed copies see it, and adds an entry to `CHANGELOG.md`.

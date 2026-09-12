# Contributing

Thanks for taking the time. This page covers what you need to open a useful issue or pull request; the conventions this repository actually enforces live in [`AGENTS.md`](AGENTS.md), and the day-to-day mechanics in the [development handbook](docs/development/automation.md).

## Getting set up

Follow the [README](README.md). The shortest path to a running system is `task clean-mode`, which needs neither Docker nor API keys.

## Opening an issue

Say what you did, what you expected, and what happened instead. For anything that touches a run, include the run id — traces are stored per run and that is what makes a report reproducible. Please do not open a public issue for a security problem; see [SECURITY.md](SECURITY.md).

## Opening a pull request

Run `task ci` first. It is the same deterministic, secret-free sequence CI runs, and it fails locally for the same reasons.

A few rules catch most review comments before they happen:

- **A new test has to be proven to fail.** Break the line it guards, watch the test go red, put it back. A green test that never failed proves nothing, and this repository asks for that evidence explicitly.
- **Generated files are not edited by hand.** Change the source in `contracts/` or `db/queries/`, then regenerate with `task gen`. `task gen:check` fails the build when the two disagree.
- **Comments are for hard algorithms only**, at most a few lines. Intent belongs in names; history, rationale and measurements belong in the commit message. A linter enforces this.
- **Secrets never enter the repository** — not in `.env.example`, not in logs, not in traces, not in a test fixture.
- **Cross-process interfaces start in `contracts/`.** Write the schema, then implement against the generated code.

Commit messages, identifiers and code comments are written in English. Documents under `docs/` are written in Traditional Chinese.

## Scope

The project is pre-release and moves quickly. Before investing in a large change, open an issue describing the problem you want to solve — it is cheaper to agree on the shape first than to rework a finished branch.

## Licence

By contributing you agree that your contribution is licensed under the [MIT License](LICENSE).

---
name: domain-memory-cli
description: Run Domain Memory commands through the compiled Rust executable instead of the Python script. Use when a script, a host or a loop calls the read commands many times, or when no Python interpreter is available; it says which committed executable to run or how to build one, what it reads and prints, its exit codes, and when to fall back to the Python script.
---

# Domain Memory CLI

The Rust executable is the same command line as `scripts/registry_tools.py`: the same commands, flags, output and refusals, byte for byte, for every command it has taken over. It runs without a Python interpreter and answers faster than the script. Its source is [`engine/`](../../engine/README.md) in this plugin.

## Get the executable

1. The plugin carries one, built from its own source, for each of these platforms; run it from the plugin directory:

   | Platform | Executable |
   | --- | --- |
   | Linux on x86-64 (static, any distribution) | `engine/bin/linux-x86_64/domain-memory` |
   | macOS on Apple silicon or Intel | `engine/bin/macos-universal/domain-memory` |
   | Windows on x86-64 | `engine/bin/windows-x86_64/domain-memory.exe` |

   `engine/bin/manifest.json` records the SHA-256 of each one and of the source it was built from.
2. On any other platform, such as Linux on ARM, build it once from the plugin directory when `cargo` is on the path:

   ```bash
   cargo build --release --locked --manifest-path engine/Cargo.toml
   ```

   It lands at `engine/target/release/domain-memory`. `engine/rust-toolchain.toml` pins the Rust version; `rustup` installs it on the first build, and the build reads nothing outside `engine/`.
3. With neither, use `python3 scripts/registry_tools.py`. Nothing in the Registry depends on which one ran.

## Call it

Pass the arguments you would pass to the Python script, with the command first:

```bash
engine/bin/linux-x86_64/domain-memory get-context --registry-root <root> --id <context>
```

- **Input**: arguments only. No command reads standard input, and none writes a file.
- **Standard output**: the answer. JSON commands print one JSON document; `validate-policy` prints one line of text.
- **Standard error**: `ERROR: <reason>` when the command refuses, and `NOTE: <hint>` lines beside a successful answer, such as a result list that was cut short or a missing Change Package directory.
- **Exit code**: `0` answered; `1` refused, with the reason on standard error; `2` the arguments did not parse, with the command's usage on standard error. The usage names the program `registry_tools.py`, as the Python script does.
- **Help**: `<command> --help` prints the same help as the Python script.

## Commands it answers

`list-proposals`, `export`, `get-record`, `get-context`, `validate-policy`, `lookup`, `resolve-terms`, `coverage`, `analyze-boundary`, `dependents`, `verify-evidence`, `cite`, `scan-secrets`, `discover-sources`, `verify-sources`, `verify-audit`, `audit-attestations`, `verify-scm-attestation`, `verify-git-governance`, `governance-readiness`, `validate`, `refresh-indexes`, `change-coupling`, `compare-registry`, `probe`, `readiness`, `upsert-candidate`, `retract-candidate`, `promote-candidate`, `demote-local-reviews`, `recover-registry-update`, `quality-gates`, `boundary-impact`, `counterfactual`, `brief`, `init-domain-memory`, `confirm-sources`, `refresh-sources`, `refresh-listing`, `refine-sources`, `amend-policy`, `validate-contract`, `reconcile-audit`, `init-signing-key`, `install-git-hitl-hook` and `install-pre-commit-hook`.

Every other command parses its arguments, then exits `1` with `ERROR: <command> is not available in the Rust engine yet`. Run the same arguments with `python3 scripts/registry_tools.py`; never treat that refusal as an answer about the Registry. Each command and its guarantees are in [the file-backed API](../../references/script-api.md).

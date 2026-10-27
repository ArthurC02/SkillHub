# Domain Memory engine

The plugin's commands, rewritten in Rust as one executable, `domain-memory`. It takes over from `scripts/registry_tools.py` one command at a time; the [domain-memory-cli](../skills/domain-memory-cli/SKILL.md) Skill tells an Agent how to build and call it. When every command is here, the Python script shrinks to nothing, and MCP becomes the same executable's `mcp-serve` speaking JSON-RPC over stdio.

## Rules

- The Python script defines the behaviour until a command moves. The engine reproduces its output, not a version of its own: a change to the script makes the parity check below fail until the engine follows.
- A command counts as moved when every output branch it has — the answer, each refusal, each read failure — has a parity case and matches. Run a new case against the script first, to see that it reaches the branch it names.
- A command that has not moved parses its arguments, then exits 1 with `ERROR: <command> is not available in the Rust engine yet`.
- The engine uses the standard library only; `Cargo.lock` stays without dependencies.

## The command-line contract

`contract/cli.json` is extracted from the script's argparse by `parity/extract_cli_contract.py`: each command's options, types, choices, defaults, mutually exclusive groups, and its rendered usage and help. It is extracted with Python 3.13, program name `registry_tools.py` and 80 columns, because argparse words its errors differently across Python versions and wraps help to the terminal width; the engine can match one of them. `--check` compares without writing, and CI uses it to catch drift.

The engine parses against that contract the way argparse does: unique prefixes of long options, `--option=value`, negative numbers as values, `-h` followed by letters still printing help, type and choice errors, required options and exclusive groups. A usage error prints the command's usage and `<prog>: error: <message>` and exits 2; `counterfactual` answers a usage error as JSON and exits 3.

Known differences: help always wraps at 80 columns, where the script follows the terminal; a lone surrogate escape in JSON becomes U+FFFD.

## Parity

`parity/parity.py` runs the same arguments through the script and the engine and compares stdout, stderr and the exit code byte for byte; run it with Python 3.13. Each command gets a generated `-h` case and an unknown-option case, and `parity/cases.json` holds the written ones. A case may build files in a temporary directory with `files`, which `{fixture}` in its arguments names; `git: true` commits the fixture as a repository; `env` sets environment variables. A run that does not finish in 120 seconds counts as a mismatch.

Python's `\d` follows Python 3.13's Unicode data; the table is `src/digits.rs`. Search uses NFKC, case folding and word characters from `src/unicode_data.rs`, which `parity/generate_unicode.py` generates with Python 3.13; regenerate it, never edit it, when the Python version changes.

## Committed binaries

`bin/` holds an executable per platform, committed so a copy of the plugin runs it without a Rust toolchain: `linux-x86_64/domain-memory`, static against musl; `macos-universal/domain-memory`, one file for Apple silicon and Intel, ad-hoc signed; and `windows-x86_64/domain-memory.exe`, which needs only the C runtime every supported Windows ships. `bin/manifest.json` records each one's SHA-256 and the SHA-256 of the source that built it: `Cargo.toml`, `Cargo.lock`, `rust-toolchain.toml`, `contract/cli.json` and `src/`.

Every change to that source rebuilds all three in the same commit:

```sh
rustup target add x86_64-unknown-linux-musl x86_64-pc-windows-gnullvm aarch64-apple-darwin x86_64-apple-darwin
pip install cargo-zigbuild ziglang
python3 build_binaries.py
```

One machine builds all three: the Linux binary links with the `rust-lld` that ships with Rust, and the macOS one with `cargo-zigbuild`, whose zig carries the macOS system library stubs, so no Apple SDK is needed. Where `python3` is not the interpreter that holds `ziglang`, point `CARGO_ZIGBUILD_ZIG_PATH` at its `zig` executable. A run whose binaries already match the source builds nothing. The Linux and Windows builds are byte-for-byte reproducible; the macOS linker writes a random `LC_UUID`, so a macOS rebuild differs in that and in the signature hash over it.

`python3 build_binaries.py --check` compares without building; the plugin's test suite runs it, and CI runs the parity cases against the committed binary on Linux, macOS and Windows.

## Commands

From this directory; `rust-toolchain.toml` pins the Rust version:

```sh
cargo fmt --check
cargo clippy --all-targets --locked -- -D warnings
cargo test --locked
cargo build --release --locked
python3.13 parity/extract_cli_contract.py --check
python3.13 parity/parity.py
python3 build_binaries.py --check
```

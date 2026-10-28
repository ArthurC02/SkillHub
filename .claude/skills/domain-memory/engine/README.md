# Domain Memory engine

The plugin's commands, rewritten in Rust as one executable, `domain-memory`. It answers every command of `scripts/registry_tools.py`, including `mcp-serve`, which speaks JSON-RPC over stdio; [running the commands](../AGENTS.md#running-the-commands) tells an Agent which executable to call. The script stays only until the Python is removed; the engine is held to the script's output recorded in `parity/expected/`.

## Rules

- The recorded output defines the behaviour. A change of behaviour changes the engine and the recording in the same commit, and the diff of the recording is what a reviewer reads.
- Every output branch a command has — the answer, each refusal, each read failure — has a case.
- The engine uses the standard library only; `Cargo.lock` stays without dependencies.

## The command-line contract

`contract/cli.json` is extracted from the script's argparse by `parity/extract_cli_contract.py`: each command's options, types, choices, defaults, mutually exclusive groups, and its rendered usage and help. It is extracted with Python 3.13, program name `registry_tools.py` and 80 columns, because argparse words its errors differently across Python versions and wraps help to the terminal width; the engine can match one of them. `--check` compares without writing, and CI uses it to catch drift.

The engine parses against that contract the way argparse does: unique prefixes of long options, `--option=value`, negative numbers as values, `-h` followed by letters still printing help, type and choice errors, required options and exclusive groups. A usage error prints the command's usage and `<prog>: error: <message>` and exits 2; `counterfactual` answers a usage error as JSON and exits 3.

Known differences: help always wraps at 80 columns, where the script follows the terminal; a lone surrogate escape in JSON becomes U+FFFD.

## Expected output

`tests/expected_output.rs` runs every case through the executable and compares stdout, stderr, the exit code and the files a case writes with `parity/expected/posix.json`, overlaid on Windows by `parity/expected/windows.json`, which holds only the cases whose output differs there. Each command gets a generated `-h` case and an unknown-option case, and `parity/cases.json` holds the written ones. A case may build files in a temporary directory with `files`, which `{fixture}` in its arguments names; `git: true` commits the fixture as a repository with a fixed date; `env` sets environment variables; `stdin` is fed to standard input, with `{fixture}` in it written as a JSON string body; `writes: true` also compares the files it leaves and checks each audit log it wrote with `verify-audit`. A run that does not finish in 120 seconds counts as a mismatch.

`{repo}` in a case names a clone of this repository at the commit the recording names as `pin`, so a case that reads the repository's own Registry or history reads the same bytes on every machine; it runs with that clone as its working directory, and CI fetches the full history for it. Before comparing, the fixture, its parent, the clone and the plugin directory become `{fixture}`, `{scratch}`, `{repo}` and `{plugin}` in each spelling a path takes in the output, the version in `mcp-serve`'s server info becomes `{version}`, and an operating-system error, `[Errno N] …` or `[WinError N] …`, becomes `[OS error] ~` up to the next `: `, `. `, quote or line end, because its number and wording belong to the system and its display language. A case that passes another program's words through, such as git's usage or ssh-keygen's reason, lists in `tool_output_after` the text after which a stream is compared as `~`. Output that a secret scanner would report is stored as `{"hex": …}`.

```sh
cargo test --release --locked --test expected_output
DOMAIN_MEMORY_BINARY=bin/linux-x86_64/domain-memory cargo test --release --locked --test expected_output -- --only "mcp-serve"
```

Without `DOMAIN_MEMORY_BINARY` it runs the executable Cargo just built; `--only` keeps the cases whose name contains the text. `parity/parity.py --record <pin>` wrote the recording from the Python script, and `parity/parity.py` without options still compares the script and the engine directly, until the script is removed.

Python's `\d` follows Python 3.13's Unicode data; the table is `src/digits.rs`. Search uses NFKC, case folding and word characters from `src/unicode_data.rs`, which `parity/generate_unicode.py` generates with Python 3.13; regenerate it, never edit it, when the Python version changes.

## Committed binaries

`bin/` holds an executable per platform, committed so a copy of the plugin runs it without a Rust toolchain: `linux-x86_64/domain-memory`, static against musl; `macos-universal/domain-memory`, one file for Apple silicon and Intel, ad-hoc signed; and `windows-x86_64/domain-memory.exe`, which needs only the C runtime every supported Windows ships. `bin/manifest.json` records each one's SHA-256 and the SHA-256 of the source that built it: `Cargo.toml`, `Cargo.lock`, `rust-toolchain.toml`, `contract/cli.json`, `src/`, and the plugin's `templates/` and `.claude-plugin/plugin.json`, which the engine embeds.

Every change to that source rebuilds all three in the same commit:

```sh
rustup target add x86_64-unknown-linux-musl x86_64-pc-windows-gnullvm aarch64-apple-darwin x86_64-apple-darwin
pip install cargo-zigbuild ziglang
python3 build_binaries.py
```

One machine builds all three: the Linux binary links with the `rust-lld` that ships with Rust, and the macOS one with `cargo-zigbuild`, whose zig carries the macOS system library stubs, so no Apple SDK is needed. Where `python3` is not the interpreter that holds `ziglang`, point `CARGO_ZIGBUILD_ZIG_PATH` at its `zig` executable. A run whose binaries already match the source builds nothing. The Linux and Windows builds are byte-for-byte reproducible; the macOS linker writes a random `LC_UUID`, so a macOS rebuild differs in that and in the signature hash over it.

`python3 build_binaries.py --check` compares without building; the plugin's test suite runs it, and CI runs the expected-output cases against the committed binary on Linux, macOS and Windows.

## Commands

From this directory; `rust-toolchain.toml` pins the Rust version:

```sh
cargo fmt --check
cargo clippy --all-targets --locked -- -D warnings
cargo test --locked
cargo build --release --locked
python3.13 parity/extract_cli_contract.py --check
cargo test --release --locked --test expected_output
python3 build_binaries.py --check
```

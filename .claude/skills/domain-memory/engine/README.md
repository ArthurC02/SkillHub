# Domain Memory engine

The plugin's commands as one executable, `domain-memory`, including `mcp-serve`, which speaks JSON-RPC over stdio; [running the commands](../AGENTS.md#running-the-commands) tells an Agent which executable to call. It began as a port of a Python script and is held to the output recorded in `parity/expected/`.

## Rules

- The recorded output defines the behaviour. A change of behaviour changes the engine and the recording in the same commit, and the diff of the recording is what a reviewer reads.
- Every output branch a command has — the answer, each refusal, each read failure — has a case.
- The engine uses the standard library only; `Cargo.lock` stays without dependencies.

## The command-line contract

`contract/cli.json` defines the command line: each command's options, types, choices, defaults, mutually exclusive groups, and its usage and help as printed, wrapped at 80 columns. It is the source, edited by hand: a new option changes its entry in `actions`, its `usage` and its `help` together, and `tests/docs.rs` holds the contract, `main.rs` and the command reference to the same commands.

The engine parses against that contract the way Python's argparse does: unique prefixes of long options, `--option=value`, negative numbers as values, `-h` followed by letters still printing help, type and choice errors, required options and exclusive groups. A usage error prints the command's usage and `<prog>: error: <message>` and exits 2; `counterfactual` answers a usage error as JSON and exits 3.

Help always wraps at 80 columns, whatever the terminal; a lone surrogate escape in JSON becomes U+FFFD.

## Expected output

`tests/expected_output.rs` runs every case through the executable and compares stdout, stderr, the exit code and the files a case writes with `parity/expected/posix.json`, overlaid on Windows by `parity/expected/windows.json`, which holds only the cases whose output differs there. Each command gets a generated `-h` case and an unknown-option case, and `parity/cases.json` holds the written ones. A case may build files in a temporary directory with `files`, which `{fixture}` in its arguments names; `git: true` commits the fixture as a repository with a fixed date; `env` sets environment variables; `stdin` is fed to standard input, with `{fixture}` in it written as a JSON string body; `writes: true` also compares the files it leaves and checks each audit log it wrote with `verify-audit`. Git gets a 300-second deadline unless the case sets `DOMAIN_MEMORY_GIT_TIMEOUT` itself. A run that does not finish in 120 seconds counts as a mismatch.

Before any case runs, each policy file a case writes, `domain-memory-policy.json` or the file its `--policy` names, goes through `validate-policy`; the run stops, and `--bless` writes nothing, when one holds a key the policy does not define. When the policy drops a key, every case that still writes it stops at the same refusal before it reaches what it tests, and the recording would accept that refusal as the answer. The cases that test an unknown key on purpose are listed by name in `CASES_ABOUT_UNKNOWN_POLICY_KEYS`.

`{repo}` in a case names a clone of this repository at the commit the recording names as `pin`, so a case that reads the repository's own Registry or history reads the same bytes on every machine; it runs with that clone as its working directory, and CI fetches the full history for it. Before comparing, the fixture, its parent, the clone and the plugin directory become `{fixture}`, `{scratch}`, `{repo}` and `{plugin}` in each spelling a path takes in the output, the version in `mcp-serve`'s server info becomes `{version}`, and an operating-system error, `[Errno N] …` or `[WinError N] …`, becomes `[OS error] ~` up to the next `: `, `. `, quote or line end, because its number and wording belong to the system and its display language. A case that passes another program's words through, such as git's usage or ssh-keygen's reason, lists in `tool_output_after` the text after which a stream is compared as `~`. Output that a secret scanner would report is stored as `{"hex": …}`. A stream or file larger than 64 KiB is stored as `{"bytes", "head", "sha256"}`: its length, its whole lines within the first 4 KiB, and its digest; a mismatch shows the first differing line of the head, or both digests when the head agrees.

```sh
cargo test --release --locked --test expected_output
DOMAIN_MEMORY_BINARY=bin/linux-x86_64/domain-memory cargo test --release --locked --test expected_output -- --only "mcp-serve"
cargo test --release --locked --test expected_output -- --bless
```

Without `DOMAIN_MEMORY_BINARY` it runs the executable Cargo just built; `--only` keeps the cases whose name contains the text. `--bless` writes what the executable printed into the recording instead of comparing: on Linux and macOS into `posix.json`, on Windows into `windows.json` for the cases whose output differs from `posix.json`. It refuses when an audit log a case wrote fails `verify-audit`; review the diff before committing it. To move `pin`, change it in `posix.json`, bless on Linux or macOS, then bless on Windows.

`\d` in a pattern, search's NFKC, case folding and word characters follow Unicode 15.1, the data the recorded behaviour was taken with; the tables are `src/digits.rs` and `src/unicode_data.rs`. They are data: a newer Unicode version replaces them whole, and the recording with them.

## Committed binaries

`bin/` holds an executable per platform, committed so a copy of the plugin runs it without a Rust toolchain: `linux-x86_64/domain-memory`, static against musl; `macos-universal/domain-memory`, one file for Apple silicon and Intel, ad-hoc signed; and `windows-x86_64/domain-memory.exe`, which needs only the C runtime every supported Windows ships. `bin/manifest.json` records each one's SHA-256 and the SHA-256 of the source that built it: `Cargo.toml`, `Cargo.lock`, `rust-toolchain.toml`, `contract/cli.json`, `src/`, and the plugin's `templates/` and `.claude-plugin/plugin.json`, which the engine embeds.

Every change to that source rebuilds all three in the same commit:

```sh
rustup target add x86_64-unknown-linux-musl x86_64-pc-windows-gnullvm aarch64-apple-darwin x86_64-apple-darwin
cargo install --locked cargo-zigbuild
cargo run --locked --example build_binaries
```

One machine builds all three: the Linux binary links with the `rust-lld` that ships with Rust, and the macOS one with `cargo-zigbuild`, whose zig carries the macOS system library stubs, so no Apple SDK is needed. `cargo-zigbuild` needs `zig` on the path (from ziglang.org or a package manager), or `CARGO_ZIGBUILD_ZIG_PATH` naming it. A run whose binaries already match the source builds nothing. The Linux and Windows builds are byte-for-byte reproducible; the macOS linker writes a random `LC_UUID`, so a macOS rebuild differs in that and in the signature hash over it.

`cargo run --locked --example build_binaries -- --check` compares without building, including the manifest's own text; CI runs it, and runs the expected-output cases against the committed binary on Linux, macOS and Windows.

## Commands

From this directory; `rust-toolchain.toml` pins the Rust version:

```sh
cargo fmt --check
cargo clippy --all-targets --locked -- -D warnings
cargo test --locked
cargo build --release --locked
cargo test --release --locked --test expected_output
cargo run --locked --example build_binaries -- --check
```

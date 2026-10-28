#[allow(dead_code)]
#[path = "../src/sha256.rs"]
mod sha256;

#[allow(dead_code, unused_imports)]
#[path = "../src/json.rs"]
mod json;

mod okf {
    pub const INT_MAX_STR_DIGITS: usize = 4300;
}

use json::{Object, Value};
use std::path::{Path, PathBuf};
use std::process::Command;

const FORMAT: &str = "domain-memory-engine-binaries/v1";
const SOURCES: [&str; 4] = [
    "Cargo.toml",
    "Cargo.lock",
    "rust-toolchain.toml",
    "contract/cli.json",
];
const REBUILD: &str = "run cargo run --example build_binaries";

struct Platform {
    name: &'static str,
    target: &'static str,
    built: &'static str,
    command: [&'static str; 2],
    environment: &'static [(&'static str, &'static str)],
}

const PLATFORMS: [Platform; 3] = [
    Platform {
        name: "linux-x86_64/domain-memory",
        target: "x86_64-unknown-linux-musl",
        built: "domain-memory",
        command: ["cargo", "build"],
        environment: &[("CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER", "rust-lld")],
    },
    Platform {
        name: "macos-universal/domain-memory",
        target: "universal2-apple-darwin",
        built: "domain-memory",
        command: ["cargo-zigbuild", "build"],
        environment: &[],
    },
    Platform {
        name: "windows-x86_64/domain-memory.exe",
        target: "x86_64-pc-windows-gnullvm",
        built: "domain-memory.exe",
        command: ["cargo", "build"],
        environment: &[(
            "CARGO_TARGET_X86_64_PC_WINDOWS_GNULLVM_RUSTFLAGS",
            "-C link-arg=--no-insert-timestamp",
        )],
    },
];

fn engine() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn plugin() -> PathBuf {
    engine()
        .parent()
        .expect("the plugin holds the engine")
        .to_path_buf()
}

fn files_under(directory: &Path, found: &mut Vec<PathBuf>) {
    let entries = std::fs::read_dir(directory)
        .unwrap_or_else(|error| panic!("cannot list {}: {error}", directory.display()));
    for entry in entries {
        let path = entry.expect("directory entry").path();
        if path.is_dir() {
            files_under(&path, found);
        } else if path.is_file() {
            found.push(path);
        }
    }
}

fn relative(path: &Path) -> String {
    path.strip_prefix(plugin())
        .expect("a source inside the plugin")
        .components()
        .map(|part| part.as_os_str().to_string_lossy().into_owned())
        .collect::<Vec<_>>()
        .join("/")
}

fn read(path: &Path) -> Vec<u8> {
    std::fs::read(path).unwrap_or_else(|error| panic!("cannot read {}: {error}", path.display()))
}

fn source_sha256() -> String {
    let mut paths: Vec<PathBuf> = SOURCES.iter().map(|name| engine().join(name)).collect();
    paths.push(plugin().join(".claude-plugin").join("plugin.json"));
    files_under(&engine().join("src"), &mut paths);
    files_under(&plugin().join("templates"), &mut paths);
    let mut named: Vec<(String, PathBuf)> = paths
        .into_iter()
        .map(|path| (relative(&path), path))
        .collect();
    named.sort();
    let mut digest = sha256::Sha256::start();
    for (name, path) in named {
        digest.update(name.as_bytes());
        digest.update(b"\0");
        digest.update(&read(&path));
        digest.update(b"\0");
    }
    digest.hex_finish()
}

fn bin() -> PathBuf {
    engine().join("bin")
}

fn expected_manifest() -> String {
    let mut binaries = Object::new();
    for platform in &PLATFORMS {
        let path = bin().join(platform.name);
        if path.is_file() {
            let mut entry = Object::new();
            entry.insert("target", Value::Str(platform.target.to_string()));
            entry.insert("sha256", Value::Str(sha256::hex_digest(&read(&path))));
            binaries.insert(platform.name, Value::Object(entry));
        }
    }
    let mut manifest = Object::new();
    manifest.insert("format", Value::Str(FORMAT.to_string()));
    manifest.insert("source_sha256", Value::Str(source_sha256()));
    manifest.insert("binaries", Value::Object(binaries));
    let style = json::Style {
        indent: Some(2),
        sort_keys: false,
        ensure_ascii: true,
        compact: false,
    };
    json::dumps(&Value::Object(manifest), style) + "\n"
}

fn recorded(manifest: &Value, path: &[&str]) -> Option<String> {
    let mut value = manifest;
    for key in path {
        value = value.as_object()?.get(key)?;
    }
    value.as_str().map(str::to_string)
}

fn problems() -> Vec<String> {
    let text = std::fs::read_to_string(bin().join("manifest.json")).unwrap_or_default();
    let Ok(manifest) = json::loads(&text) else {
        return vec![format!(
            "bin/manifest.json is missing or unreadable; {REBUILD}"
        )];
    };
    let mut found = Vec::new();
    if recorded(&manifest, &["source_sha256"]) != Some(source_sha256()) {
        found.push(format!(
            "the engine source changed after the binaries were built; {REBUILD}"
        ));
    }
    for platform in &PLATFORMS {
        let path = bin().join(platform.name);
        if !path.is_file() {
            found.push(format!("bin/{} is missing; {REBUILD}", platform.name));
        } else if recorded(&manifest, &["binaries", platform.name, "sha256"])
            != Some(sha256::hex_digest(&read(&path)))
        {
            found.push(format!(
                "bin/{} is not the binary the manifest records; {REBUILD}",
                platform.name
            ));
        }
    }
    if found.is_empty() && text != expected_manifest() {
        found.push(format!(
            "bin/manifest.json is not the manifest the binaries give; {REBUILD}"
        ));
    }
    found
}

fn build() {
    let source = source_sha256();
    for platform in &PLATFORMS {
        let status = Command::new(platform.command[0])
            .arg(platform.command[1])
            .args(["--release", "--locked", "--target", platform.target])
            .current_dir(engine())
            .envs(platform.environment.iter().copied())
            .status()
            .unwrap_or_else(|error| panic!("cannot run {}: {error}", platform.command[0]));
        if !status.success() {
            eprintln!("ERROR: the {} build failed", platform.target);
            std::process::exit(1);
        }
        let destination = bin().join(platform.name);
        std::fs::create_dir_all(destination.parent().expect("a platform directory"))
            .expect("platform directory");
        let built = engine()
            .join("target")
            .join(platform.target)
            .join("release")
            .join(platform.built);
        std::fs::copy(&built, &destination)
            .unwrap_or_else(|error| panic!("cannot copy {}: {error}", built.display()));
        executable(&destination);
    }
    if source_sha256() != source {
        eprintln!(
            "ERROR: the engine source changed while the binaries were building; {REBUILD} again"
        );
        std::process::exit(1);
    }
    std::fs::write(bin().join("manifest.json"), expected_manifest()).expect("manifest");
}

#[cfg(unix)]
fn executable(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o755)).expect("file mode");
}

#[cfg(not(unix))]
fn executable(_: &Path) {}

fn main() {
    let check = std::env::args().any(|arg| arg == "--check");
    if !check && !problems().is_empty() {
        build();
    }
    let found = problems();
    for problem in &found {
        eprintln!("ERROR: {problem}");
    }
    if !found.is_empty() {
        std::process::exit(1);
    }
}

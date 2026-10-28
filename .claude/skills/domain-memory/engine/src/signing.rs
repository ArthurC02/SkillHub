use std::path::{Path, PathBuf};

use crate::files::make_directories;
use crate::git::{run_bounded, universal_newlines};
use crate::json::{Object, Value};
use crate::py::{self, PurePath};

const SIGNING_KEY_ENV: &str = "DOMAIN_MEMORY_SIGNING_KEY";

struct Ran {
    code: i32,
    stdout: String,
    stderr: String,
}

fn run(argv: &[&str]) -> Result<Ran, String> {
    let done = run_bounded(argv, None).map_err(|error| error.message)?;
    Ok(Ran {
        code: done.code,
        stdout: universal_newlines(&done.stdout),
        stderr: universal_newlines(&done.stderr),
    })
}

fn default_key_file() -> Result<String, String> {
    let variable = if cfg!(windows) { "USERPROFILE" } else { "HOME" };
    let home =
        std::env::var(variable).map_err(|_| "Could not determine home directory.".to_string())?;
    Ok(PurePath::new(&home)
        .joined(".domain-memory")
        .joined("signing-key")
        .native())
}

fn written(path: &Path, text: &str) -> Result<(), String> {
    std::fs::write(path, text).map_err(|error| py::open_error(&error, path))
}

fn write_verbatim(path: &Path, text: &str) -> Result<(), String> {
    if text.ends_with('\n') {
        written(path, text)
    } else {
        written(path, &format!("{text}\n"))
    }
}

#[cfg(unix)]
fn restrict(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    let _ = std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600));
}

#[cfg(not(unix))]
fn restrict(_path: &Path) {}

#[cfg(unix)]
fn private_options(options: &mut std::fs::OpenOptions) {
    use std::os::unix::fs::OpenOptionsExt;
    options.mode(0o600);
}

#[cfg(not(unix))]
fn private_options(_options: &mut std::fs::OpenOptions) {}

fn write_private(path: &Path, text: &str) -> Result<(), String> {
    use std::io::Write;
    let mut options = std::fs::OpenOptions::new();
    options.write(true).create(true).truncate(true);
    private_options(&mut options);
    let mut output = options
        .open(path)
        .map_err(|error| py::open_error(&error, path))?;
    let full = if text.ends_with('\n') {
        text.to_string()
    } else {
        format!("{text}\n")
    };
    output
        .write_all(full.as_bytes())
        .map_err(|error| py::os_error(&error, path))
}

fn generate(key_file: &str, principal: &str) -> Result<(), String> {
    let done = run(&[
        "ssh-keygen",
        "-t",
        "ed25519",
        "-f",
        key_file,
        "-N",
        "",
        "-C",
        principal,
        "-q",
    ])?;
    if done.code != 0 || !Path::new(key_file).is_file() {
        return Err(format!(
            "ssh-keygen could not create a signing key: {}",
            py::strip(&done.stderr)
        ));
    }
    Ok(())
}

fn adopt_from_environment(key_file: &str, material: &str) -> Result<(), String> {
    let path = Path::new(key_file);
    write_private(path, &material.replace("\r\n", "\n").replace('\r', "\n"))?;
    restrict(path);
    let done = run(&["ssh-keygen", "-y", "-f", key_file])?;
    if done.code != 0 {
        match std::fs::remove_file(path) {
            Err(error) if error.kind() != std::io::ErrorKind::NotFound => {
                return Err(py::os_error(&error, path));
            }
            _ => {}
        }
        return Err(format!(
            "{SIGNING_KEY_ENV} does not hold a readable OpenSSH private key: {}",
            py::strip(&done.stderr)
        ));
    }
    write_verbatim(
        Path::new(&format!("{key_file}.pub")),
        py::strip(&done.stdout),
    )
}

fn fingerprint(key_file: &str) -> Result<String, String> {
    let done = run(&["ssh-keygen", "-lf", &format!("{key_file}.pub")])?;
    if done.code != 0 {
        return Err(format!(
            "ssh-keygen could not read the public key: {}",
            py::strip(&done.stderr)
        ));
    }
    done.stdout
        .split(py::is_space)
        .find(|field| field.starts_with("SHA256:"))
        .map(str::to_string)
        .ok_or_else(|| "ssh-keygen did not report a SHA256 fingerprint".to_string())
}

fn text(value: &str) -> Value {
    Value::Str(value.to_string())
}

pub struct SigningKey<'a> {
    pub repo_root: &'a Path,
    pub principal: &'a str,
    pub key_file: Option<&'a str>,
    pub force: bool,
    pub sign_every_commit: bool,
}

fn source_of_key(key_file: &str, force: bool, principal: &str) -> Result<&'static str, String> {
    let material = std::env::var(SIGNING_KEY_ENV).unwrap_or_default();
    let exists = Path::new(key_file).exists();
    if !py::strip(&material).is_empty() {
        if exists && !force {
            return Err(format!(
                "{key_file} already exists; pass --force to replace it from {SIGNING_KEY_ENV}"
            ));
        }
        adopt_from_environment(key_file, &material)?;
        return Ok("environment");
    }
    if exists {
        if force {
            return Err(format!(
                "--force replaces a key from {SIGNING_KEY_ENV}, and {SIGNING_KEY_ENV} is not set; \
                 delete {key_file} yourself if you mean to discard it"
            ));
        }
        return Ok("reused");
    }
    generate(key_file, principal)?;
    Ok("generated")
}

pub fn init_signing_key(request: &SigningKey) -> Result<Object, String> {
    let principal = py::strip(request.principal);
    if principal.is_empty() {
        return Err(
            "a signing key needs a principal: the identity an approval is attributed to"
                .to_string(),
        );
    }
    let key_file = match request.key_file {
        Some(given) => PurePath::new(given).native(),
        None => default_key_file()?,
    };
    let parent = PurePath::new(&key_file).parent().native();
    make_directories(Path::new(&parent))?;
    let source = source_of_key(&key_file, request.force, principal)?;
    restrict(Path::new(&key_file));
    let public_path = PathBuf::from(format!("{key_file}.pub"));
    let bytes =
        std::fs::read(&public_path).map_err(|error| py::open_error(&error, &public_path))?;
    let public_key = py::strip(&py::utf8_text(&bytes)?).to_string();
    let allowed = format!("{key_file}.allowed_signers");
    write_verbatim(Path::new(&allowed), &format!("{principal} {public_key}"))?;
    let digest = fingerprint(&key_file)?;
    let mut settings = vec![
        ("gpg.format", "ssh".to_string()),
        ("user.signingkey", key_file.clone()),
        ("gpg.ssh.allowedSignersFile", allowed.clone()),
    ];
    if request.sign_every_commit {
        settings.push(("commit.gpgsign", "true".to_string()));
    }
    let repo = py::shown(request.repo_root);
    for (name, value) in &settings {
        let done = run(&["git", "-C", &repo, "config", name, value])?;
        if done.code != 0 {
            return Err(format!(
                "git config {name} failed: {}",
                py::strip(&done.stderr)
            ));
        }
    }
    settings.sort_by_key(|(name, _)| *name);
    let mut result = Object::new();
    result.insert("format", text("domain-memory-signing-key/v1"));
    result.insert("source", text(source));
    result.insert("principal", text(principal));
    result.insert("fingerprint", text(&digest));
    result.insert("key_file", text(&key_file));
    result.insert("public_key_file", text(&format!("{key_file}.pub")));
    result.insert("allowed_signers_file", text(&allowed));
    result.insert(
        "authorized_signers",
        Value::Array(vec![text(principal), text(&digest)]),
    );
    result.insert("portable_via", text(SIGNING_KEY_ENV));
    result.insert(
        "git_config",
        Value::Array(settings.iter().map(|(name, _)| text(name)).collect()),
    );
    result.insert("signs_every_commit", Value::Bool(request.sign_every_commit));
    Ok(result)
}

#[cfg(all(test, unix))]
mod tests {
    use super::*;
    use std::os::unix::fs::PermissionsExt;

    #[test]
    fn a_key_written_from_the_environment_is_created_readable_by_its_owner_alone() {
        let root = std::env::temp_dir().join(format!("dm-signing-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(&root).unwrap();
        let key = root.join("signing-key");
        write_private(&key, "material").unwrap();
        let mode = std::fs::metadata(&key).unwrap().permissions().mode() & 0o777;
        let _ = std::fs::remove_dir_all(&root);
        assert_eq!(mode, 0o600);
    }
}

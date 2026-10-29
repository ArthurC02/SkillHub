use crate::pyvalue::{equal, message_text, splitlines};
use std::collections::HashMap;
use std::path::{Path, PathBuf};

use crate::files::{PROPOSAL_FILE, load_json};
use crate::git;
use crate::json::{self, Object, Value};
use crate::proposals::{completed_identifier, readable_packages};
use crate::py;
use crate::unicode::casefold;

const EVIDENCE_FILE: &str = "evidence-bundle.json";
const UNRUNNABLE_PROGRAM: [&str; 2] = ["cannot spawn ", "cannot run "];
const OUTSIDE_THE_REPOSITORY: &str =
    "the Change Package must be inside the repository, so the attested commit can carry it";
const AUDITED_STATUSES: [&str; 2] = ["approved", "applied"];
const COMMIT_LENGTH: usize = 40;

struct AuditedPackage {
    package: PathBuf,
    proposal: Object,
    attestation: Value,
}

fn captures(text: &str, prefix: &str, accepts: fn(char) -> bool) -> Vec<String> {
    let mut found = Vec::new();
    let mut from = 0;
    while let Some(at) = text[from..].find(prefix) {
        let start = from + at + prefix.len();
        let length: usize = text[start..]
            .chars()
            .take_while(|character| accepts(*character))
            .map(char::len_utf8)
            .sum();
        if length > 0 {
            found.push(text[start..start + length].to_string());
            from = start + length;
        } else {
            from += at + 1;
        }
    }
    found
}

fn signers_in(output: &str) -> Vec<String> {
    let mut signers = captures(output, "VALIDSIG ", |c| {
        c.is_ascii_digit() || ('A'..='F').contains(&c)
    });
    signers.extend(captures(output, "Good \"git\" signature for ", |c| {
        !py::is_space(c)
    }));
    signers.extend(
        captures(output, "key SHA256:", |c| {
            c.is_ascii_alphanumeric() || matches!(c, '+' | '/' | '=')
        })
        .into_iter()
        .map(|fingerprint| format!("SHA256:{fingerprint}")),
    );
    signers
}

fn invalid_signature(
    repo_root: &Path,
    commit: &str,
    verifier_stderr: &str,
) -> Result<String, String> {
    let reasons: Vec<&str> = splitlines(verifier_stderr)
        .into_iter()
        .filter(|line| !py::strip(line).is_empty())
        .map(|line| line.strip_prefix("error: ").unwrap_or(line))
        .collect();
    let unrunnable = reasons.iter().find(|reason| {
        UNRUNNABLE_PROGRAM
            .iter()
            .any(|prefix| reason.starts_with(prefix))
    });
    if let Some(unrunnable) = unrunnable {
        return Ok(format!(
            "Git commit signature was not checked: {unrunnable}. Install the signing program, \
             or point gpg.ssh.program or gpg.program at it, and verify again"
        ));
    }
    let commit_object = git::run_git_text(repo_root, &["cat-file", "commit", commit])?;
    let header = commit_object
        .stdout
        .split_once("\n\n")
        .map_or(commit_object.stdout.as_str(), |(header, _)| header);
    if header.contains("\ngpgsig") {
        let reason = reasons
            .last()
            .map_or(String::new(), |last| format!(" ({last})"));
        return Ok(format!(
            "Git commit signature is invalid: {commit} is signed, but the signature does not verify{reason}"
        ));
    }
    Ok(format!(
        "Git commit signature is invalid: {commit} carries no signature. A rebase, amend or \
         cherry-pick writes new commits unsigned unless told to sign; take in upstream work with a \
         merge (git pull --no-rebase), which keeps signed and attested commits as they are, and \
         an attestation naming a commit that was rewritten has to be made again"
    ))
}

pub fn verify_git_signed_commit(
    commit: &str,
    repo_root: &Path,
    registry_root: &Path,
    authorized_signers: &[String],
    distinct_signer: bool,
) -> Result<Vec<String>, String> {
    let verified = git::run_git_text(repo_root, &["verify-commit", "--raw", commit])?;
    if verified.code != 0 {
        return Ok(vec![invalid_signature(
            repo_root,
            commit,
            &verified.stderr,
        )?]);
    }
    let output = format!("{}{}", verified.stdout, verified.stderr);
    if !signers_in(&output)
        .iter()
        .any(|signer| authorized_signers.contains(signer))
    {
        return Ok(vec![
            "Git commit signer is not authorized by Domain Memory policy".to_string(),
        ]);
    }
    if distinct_signer && let Some(same) = author_is_signer(repo_root, commit)? {
        return Ok(vec![same]);
    }
    let changed = git::run_git_text(
        repo_root,
        &[
            "diff-tree",
            "--no-commit-id",
            "--name-only",
            "-r",
            "-m",
            "--first-parent",
            "--root",
            commit,
        ],
    )?;
    let root = relative_posix(
        &py::resolve_path(registry_root),
        &py::resolve_path(repo_root),
    )?;
    let inside = splitlines(&changed.stdout)
        .into_iter()
        .any(|path| path == root || path.starts_with(&format!("{root}/")));
    if changed.code != 0 || !inside {
        return Ok(vec![
            "Git commit does not change Domain Memory files".to_string(),
        ]);
    }
    Ok(Vec::new())
}

fn author_is_signer(repo_root: &Path, commit: &str) -> Result<Option<String>, String> {
    let logged = git::run_git_text(repo_root, &["log", "-1", "--format=%ae%n%GS", commit])?;
    let identities = splitlines(&logged.stdout);
    let identity = |index: usize| identities.get(index).map_or("", |line| py::strip(line));
    let (author, signer) = (identity(0), identity(1));
    if author.is_empty() || signer.is_empty() {
        return Ok(Some(format!(
            "Git commit {commit} author or signer identity could not be read, so a signer other than the author cannot be confirmed; the policy requires distinct_signer"
        )));
    }
    if casefold(signer).contains(&casefold(author)) {
        return Ok(Some(format!(
            "Git commit author {author} is also its signer; the policy requires distinct_signer, a reviewer other than the author signing the commit that carries the package"
        )));
    }
    Ok(None)
}

pub fn verify_attested_commit(
    attestation: &Object,
    repo_root: &Path,
    registry_root: &Path,
    governance: &Object,
) -> Result<Vec<String>, String> {
    let Some(commit) = attestation.get("commit").and_then(Value::as_str) else {
        return Ok(vec!["Git commit attestation requires a commit".to_string()]);
    };
    let signers: Vec<String> = governance
        .get("authorized_signers")
        .and_then(Value::as_array)
        .unwrap_or_default()
        .iter()
        .filter_map(|signer| signer.as_str().map(str::to_string))
        .collect();
    let distinct = governance
        .get("distinct_signer")
        .is_some_and(crate::okf::truthy);
    verify_git_signed_commit(commit, repo_root, registry_root, &signers, distinct)
}

fn relative_posix(path: &Path, top: &Path) -> Result<String, String> {
    let relative = py::relative_to(path, top)?;
    Ok(if relative.is_empty() {
        ".".to_string()
    } else {
        relative
    })
}

pub fn package_path_in(repo_root: &Path, package_root: &Path) -> Option<String> {
    let top = py::resolve_path(repo_root);
    let located = py::resolve_path(&top.join(package_root));
    relative_posix(&located, &top).ok()
}

pub fn commit_carries_proposal(
    repo_root: &Path,
    commit: &str,
    package_root: &Path,
    proposal: &Object,
) -> Result<Vec<String>, String> {
    let Some(relative) = package_path_in(repo_root, package_root) else {
        return Ok(vec![OUTSIDE_THE_REPOSITORY.to_string()]);
    };
    let shown = git::run_git_text(
        repo_root,
        &["show", &format!("{commit}:{relative}/{PROPOSAL_FILE}")],
    )?;
    Ok(carried_proposal_errors(
        (shown.code == 0).then_some(shown.stdout.as_str()),
        &relative,
        proposal,
    ))
}

pub fn carried_proposal_errors(
    committed_text: Option<&str>,
    relative: &str,
    proposal: &Object,
) -> Vec<String> {
    let Some(committed_text) = committed_text else {
        return vec![format!(
            "Git commit does not carry this Change Package at {relative}; the attested commit \
             must contain the package, so its review covers this proposal"
        )];
    };
    let Ok(committed) = json::parse_keeping_last_duplicate(committed_text) else {
        return vec!["Git commit carries an unreadable domain-change-proposal.json".to_string()];
    };
    let same_identity = committed.as_object().is_some_and(|object| {
        ["proposal_id", "proposal_revision"].iter().all(|key| {
            equal(
                object.get(key).unwrap_or(&Value::Null),
                proposal.get(key).unwrap_or(&Value::Null),
            )
        })
    });
    if same_identity {
        return Vec::new();
    }
    vec![
        "Git commit carries another proposal or revision; attest a commit with this \
         proposal_id and proposal_revision"
            .to_string(),
    ]
}

fn attested_commit(attestation: &Value) -> Option<&str> {
    attestation
        .as_object()
        .and_then(|object| object.get("commit"))
        .and_then(Value::as_str)
}

fn proposal_object(commit: &str, relative: &str) -> String {
    format!("{commit}:{relative}/{PROPOSAL_FILE}")
}

fn read_objects(repo_root: &Path, names: &[String]) -> Result<HashMap<String, Vec<u8>>, String> {
    let mut asked: Vec<&String> = Vec::new();
    for name in names.iter().filter(|name| !name.contains('\n')) {
        if !asked.contains(&name) {
            asked.push(name);
        }
    }
    if asked.is_empty() {
        return Ok(HashMap::new());
    }
    let request: String = asked.iter().map(|name| format!("{name}\n")).collect();
    let done = git::run_git_input(
        repo_root,
        &["cat-file", "--batch"],
        Some(request.as_bytes()),
    )?;
    if done.code != 0 {
        return Ok(HashMap::new());
    }
    batch_objects(&done.stdout, &asked)
}

fn batch_objects(output: &[u8], asked: &[&String]) -> Result<HashMap<String, Vec<u8>>, String> {
    let mut found = HashMap::new();
    let mut offset = 0;
    for name in asked {
        let header_end = output[offset.min(output.len())..]
            .iter()
            .position(|byte| *byte == b'\n')
            .map(|at| offset + at)
            .ok_or("subsection not found")?;
        let header = &output[offset..header_end];
        let last_field = header.rsplit(|byte| *byte == b' ').next().unwrap_or(header);
        offset = header_end + 1;
        if !last_field.is_empty() && last_field.iter().all(u8::is_ascii_digit) {
            let size: usize = String::from_utf8_lossy(last_field)
                .parse()
                .unwrap_or(usize::MAX);
            let end = offset.saturating_add(size).min(output.len());
            found.insert((*name).clone(), output[offset.min(end)..end].to_vec());
            offset = offset.saturating_add(size).saturating_add(1);
        }
    }
    Ok(found)
}

fn attestation_verdict(
    repo_root: &Path,
    audited: &AuditedPackage,
    objects: &HashMap<String, Vec<u8>>,
) -> (&'static str, String) {
    let Some(commit) = attested_commit(&audited.attestation) else {
        return (
            "not carried",
            "the package has no scm_attestation commit".to_string(),
        );
    };
    let relative = package_path_in(repo_root, &audited.package);
    if !objects.contains_key(&format!("{commit}^{{commit}}")) {
        return (
            "unconfirmed",
            format!("commit {commit} is not in this clone; fetch the full history"),
        );
    }
    let errors = match relative {
        None => vec![OUTSIDE_THE_REPOSITORY.to_string()],
        Some(relative) => {
            let shown = objects.get(&proposal_object(commit, &relative));
            let text = shown.map(|bytes| String::from_utf8_lossy(bytes).into_owned());
            carried_proposal_errors(text.as_deref(), &relative, &audited.proposal)
        }
    };
    match errors.into_iter().next() {
        None => ("carried", commit.to_string()),
        Some(first) => ("not carried", first),
    }
}

fn audited_packages(changes_root: &Path) -> Result<Vec<AuditedPackage>, String> {
    let mut audited = Vec::new();
    for (package, proposal) in readable_packages(changes_root)? {
        let status = proposal.get("status").unwrap_or(&Value::Null);
        if matches!(status, Value::Array(_) | Value::Object(_)) {
            return Err(format!(
                "unhashable type: '{}'",
                if matches!(status, Value::Array(_)) {
                    "list"
                } else {
                    "dict"
                }
            ));
        }
        if !status
            .as_str()
            .is_some_and(|text| AUDITED_STATUSES.contains(&text))
        {
            continue;
        }
        let evidence_file = package.join(EVIDENCE_FILE);
        let attestation = if evidence_file.is_file() {
            load_json(&evidence_file)?
                .get("scm_attestation")
                .cloned()
                .unwrap_or(Value::Null)
        } else {
            Value::Null
        };
        audited.push(AuditedPackage {
            package,
            proposal,
            attestation,
        });
    }
    Ok(audited)
}

pub fn audit_attestations(repo_root: &Path, changes_root: &Path) -> Result<Vec<Value>, String> {
    if !changes_root.is_dir() {
        return Err(format!(
            "no Change Packages directory at {}; pass the directory that holds them",
            py::shown(changes_root)
        ));
    }
    let audited = audited_packages(changes_root)?;
    let mut names = Vec::new();
    for entry in &audited {
        if let Some(commit) = attested_commit(&entry.attestation) {
            names.push(format!("{commit}^{{commit}}"));
            if let Some(relative) = package_path_in(repo_root, &entry.package) {
                names.push(proposal_object(commit, &relative));
            }
        }
    }
    let objects = read_objects(repo_root, &names)?;
    Ok(audited
        .iter()
        .map(|entry| {
            let (verdict, detail) = attestation_verdict(repo_root, entry, &objects);
            let field = |key: &str| message_text(entry.proposal.get(key).unwrap_or(&Value::Null));
            let mut finding = Object::new();
            finding.insert(
                "package",
                Value::Str(
                    package_path_in(repo_root, &entry.package)
                        .unwrap_or_else(|| py::shown(&entry.package)),
                ),
            );
            finding.insert("proposal_id", Value::Str(field("proposal_id")));
            finding.insert("status", Value::Str(field("status")));
            finding.insert("verdict", Value::Str(verdict.to_string()));
            finding.insert("detail", Value::Str(detail));
            Value::Object(finding)
        })
        .collect())
}

fn valid_commit(value: &Value) -> bool {
    value.as_str().is_some_and(|text| {
        text.len() == COMMIT_LENGTH
            && text
                .bytes()
                .all(|byte| matches!(byte, b'0'..=b'9' | b'a'..=b'f'))
    })
}

pub fn verify_scm_value(value: &Object, proposal: &Object) -> Vec<String> {
    let field = |object: &Object, key: &str| object.get(key).cloned().unwrap_or(Value::Null);
    let mut errors: Vec<String> = ["provider", "commit", "status"]
        .iter()
        .filter(|key| !completed_identifier(&field(value, key)))
        .map(|key| format!("SCM attestation requires {key}"))
        .collect();
    if field(value, "status").as_str() != Some("approved") {
        errors.push("SCM attestation status must be approved".to_string());
    }
    if field(value, "provider").as_str() != Some("git-signed-commit") {
        errors.push("SCM attestation provider must be git-signed-commit".to_string());
    }
    if !valid_commit(&field(value, "commit")) {
        errors.push("SCM attestation requires a pinned 40-character commit".to_string());
    }
    if !equal(
        &field(value, "proposal_revision"),
        &field(proposal, "proposal_revision"),
    ) {
        errors.push("SCM attestation proposal revision does not match".to_string());
    }
    if !equal(
        &field(value, "base_registry_revision"),
        &field(proposal, "base_registry_revision"),
    ) {
        errors.push("SCM attestation Registry revision does not match".to_string());
    }
    errors
}

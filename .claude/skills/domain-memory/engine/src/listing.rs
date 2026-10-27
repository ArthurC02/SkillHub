use std::collections::HashSet;
use std::fs;
use std::path::Path;

use crate::git::{self, Failure};
use crate::py;

pub const EXCLUDED_DIRECTORIES: [&str; 9] = [
    ".git",
    ".hg",
    ".svn",
    "node_modules",
    ".venv",
    "vendor",
    "dist",
    "build",
    "generated",
];
const TRACKED_PREFILTER_MIN_PATHS: usize = 512;
const INDEX_TAGS_GIT_NEVER_IGNORES: [&str; 2] = ["H ", "M "];

pub struct Files {
    pub entries: Vec<String>,
    pub pruned_by_git: bool,
}

pub fn listing_of(root: &Path, excluded: &HashSet<String>) -> Result<Files, String> {
    let ignored = git_ignored_entries(root)?;
    let mut entries = Vec::new();
    let none = HashSet::new();
    collect_files(
        root,
        "",
        excluded,
        ignored.as_ref().unwrap_or(&none),
        &mut entries,
    );
    Ok(Files {
        entries,
        pruned_by_git: ignored.is_some(),
    })
}

fn git_ignored_entries(root: &Path) -> Result<Option<HashSet<String>>, String> {
    let arguments = [
        "ls-files",
        "--others",
        "--ignored",
        "--exclude-standard",
        "--directory",
        "-z",
    ];
    match git::run_git(root, &arguments, None) {
        Err(error) if error.kind == Failure::Invalid => Err(error.message),
        Err(_) => Ok(None),
        Ok(done) if done.code != 0 => Ok(None),
        Ok(done) => Ok(Some(nul_separated(&done.stdout).collect())),
    }
}

fn nul_separated(bytes: &[u8]) -> impl Iterator<Item = String> + '_ {
    bytes
        .split(|byte| *byte == 0)
        .filter(|entry| !entry.is_empty())
        .map(|entry| String::from_utf8_lossy(entry).into_owned())
}

fn collect_files(
    directory: &Path,
    prefix: &str,
    excluded: &HashSet<String>,
    ignored: &HashSet<String>,
    found: &mut Vec<String>,
) {
    let Ok(entries) = fs::read_dir(directory) else {
        return;
    };
    let listed: Vec<fs::DirEntry> = entries.flatten().collect();
    let names: Vec<String> = listed
        .iter()
        .map(|entry| entry.file_name().to_string_lossy().into_owned())
        .collect();
    if !prefix.is_empty() && names.iter().any(|name| name == ".git") {
        return;
    }
    let mut children = Vec::new();
    for (entry, name) in listed.iter().zip(&names) {
        let relative = format!("{prefix}{name}");
        if !is_directory(entry) {
            if !ignored.contains(&relative) {
                found.push(relative);
            }
        } else if !excluded.contains(name)
            && !entry.file_type().is_ok_and(|kind| kind.is_symlink())
            && !ignored.contains(&format!("{relative}/"))
        {
            children.push((entry.path(), relative));
        }
    }
    for (path, relative) in children {
        collect_files(&path, &format!("{relative}/"), excluded, ignored, found);
    }
}

fn is_directory(entry: &fs::DirEntry) -> bool {
    match entry.file_type() {
        Ok(kind) if kind.is_symlink() => fs::metadata(entry.path()).is_ok_and(|meta| meta.is_dir()),
        Ok(kind) => kind.is_dir(),
        Err(_) => false,
    }
}

pub fn ignored_among(
    root: &Path,
    files: &Files,
    relatives: &[String],
) -> Result<HashSet<String>, String> {
    if files.pruned_by_git {
        return Ok(HashSet::new());
    }
    ignored_relatives(root, relatives)
}

pub fn ignored_relatives(root: &Path, relatives: &[String]) -> Result<HashSet<String>, String> {
    if relatives.is_empty() || !root.join(".git").exists() {
        return Ok(HashSet::new());
    }
    let relatives = untracked(root, relatives)?;
    let input = relatives.join("\0");
    let done = git::run_git(
        root,
        &["check-ignore", "-z", "--stdin"],
        Some(input.as_bytes()),
    )
    .map_err(|error| error.message)?;
    if done.code != 0 && done.code != 1 {
        let stderr = String::from_utf8_lossy(&done.stderr);
        return Err(format!(
            "git check-ignore failed in {}, so ignored files cannot be told from sources: {}",
            py::shown(root),
            py::strip(&stderr)
        ));
    }
    let asked: HashSet<&String> = relatives.iter().collect();
    Ok(nul_separated(&done.stdout)
        .filter(|entry| asked.contains(entry))
        .collect())
}

fn untracked(root: &Path, relatives: &[String]) -> Result<Vec<String>, String> {
    if relatives.len() < TRACKED_PREFILTER_MIN_PATHS {
        return Ok(relatives.to_vec());
    }
    let listed = match git::run_git(root, &["ls-files", "-z", "-t"], None) {
        Err(error) if error.kind == Failure::Spawn => return Ok(relatives.to_vec()),
        Err(error) => return Err(error.message),
        Ok(done) => done,
    };
    let tracked: HashSet<String> = nul_separated(&listed.stdout)
        .filter(|entry| {
            INDEX_TAGS_GIT_NEVER_IGNORES
                .iter()
                .any(|tag| entry.starts_with(tag))
        })
        .map(|entry| entry[2..].to_string())
        .collect();
    Ok(relatives
        .iter()
        .filter(|entry| !tracked.contains(*entry))
        .cloned()
        .collect())
}

use std::fs;
use std::path::{Path, PathBuf};

use crate::git::{self, Failure};
use crate::json::{Object, Value};
use crate::secret_patterns::PATTERNS;

const UNSCANNED_DIRECTORIES: [&str; 3] = [".git", "node_modules", ".venv"];
const BINARY_PROBE_BYTES: usize = 8192;
const MAX_REPORTED_SKIPS: usize = 1_000;
const RESOURCE_LIMIT: &str = "resource-limit";

#[derive(Clone, Copy)]
pub struct Limits {
    pub files: u64,
    pub bytes: u64,
    pub file_bytes: u64,
}

pub const LIMITS: Limits = Limits {
    files: 50_000,
    bytes: 512 * 1024 * 1024,
    file_bytes: 1_048_576,
};

struct Finding {
    path: String,
    line: usize,
    kind: &'static str,
}

struct Scan {
    root: PathBuf,
    limits: Limits,
    findings: Vec<Finding>,
    skipped: Vec<(String, &'static str)>,
    skipped_count: usize,
    scanned_files: u64,
    scanned_bytes: u64,
}

impl Scan {
    fn new(root: PathBuf, limits: Limits) -> Self {
        Self {
            root,
            limits,
            findings: Vec::new(),
            skipped: Vec::new(),
            skipped_count: 0,
            scanned_files: 0,
            scanned_bytes: 0,
        }
    }

    fn skip(&mut self, relative: &str, reason: &'static str) {
        self.skipped_count += 1;
        if self.skipped.len() < MAX_REPORTED_SKIPS {
            self.skipped.push((relative.to_string(), reason));
        }
    }

    fn refusal(&mut self, path: &Path, relative: &str) -> Option<&'static str> {
        let Ok(metadata) = fs::metadata(path) else {
            return Some("unreadable");
        };
        let size = metadata.len();
        if size > self.limits.file_bytes {
            return Some("file-too-large");
        }
        if self.scanned_files + 1 > self.limits.files
            || self.scanned_bytes + size > self.limits.bytes
        {
            return Some(RESOURCE_LIMIT);
        }
        self.scanned_files += 1;
        self.scanned_bytes += size;
        self.read(path, relative).err().map(|()| "unreadable")
    }

    fn read(&mut self, path: &Path, relative: &str) -> Result<(), ()> {
        let bytes = fs::read(path).map_err(|_| ())?;
        let probe = &bytes[..bytes.len().min(BINARY_PROBE_BYTES)];
        if probe.contains(&0) {
            return Ok(());
        }
        for (index, raw) in bytes.split_inclusive(|byte| *byte == b'\n').enumerate() {
            let line = ignoring_invalid_utf8(raw);
            for (kind, matches) in PATTERNS {
                if matches(&line) {
                    self.findings.push(Finding {
                        path: relative.to_string(),
                        line: index + 1,
                        kind,
                    });
                }
            }
        }
        Ok(())
    }

    fn examine(&mut self, path: &Path, relative: &str) -> bool {
        let refused = self.refusal(path, relative);
        if let Some(reason) = refused {
            self.skip(relative, reason);
        }
        refused != Some(RESOURCE_LIMIT)
    }

    fn examine_listed(&mut self, listed: &[String]) {
        for entry in listed {
            if entry
                .split('/')
                .any(|part| UNSCANNED_DIRECTORIES.contains(&part))
            {
                continue;
            }
            let path = self.root.join(entry);
            if is_symlink(&path) {
                self.skip(entry, "symlink");
            } else if path.is_file() && !self.examine(&path, entry) {
                return;
            }
        }
    }

    fn walk(&mut self, directory: &Path, prefix: &str) -> bool {
        let Ok(entries) = fs::read_dir(directory) else {
            return true;
        };
        let (mut directories, mut files) = (Vec::new(), Vec::new());
        for entry in entries.flatten() {
            let name = entry.file_name().to_string_lossy().into_owned();
            if entry.path().is_dir() {
                directories.push(name);
            } else {
                files.push(name);
            }
        }
        let mut kept = Vec::new();
        for name in directories {
            if UNSCANNED_DIRECTORIES.contains(&name.as_str()) {
                continue;
            }
            if is_symlink(&directory.join(&name)) {
                self.skip(&format!("{prefix}{name}"), "symlink");
            } else {
                kept.push(name);
            }
        }
        for name in files {
            if UNSCANNED_DIRECTORIES.contains(&name.as_str()) {
                continue;
            }
            let path = directory.join(&name);
            let relative = format!("{prefix}{name}");
            if is_symlink(&path) {
                self.skip(&relative, "symlink");
            } else if path.is_file() && !self.examine(&path, &relative) {
                return false;
            }
        }
        kept.iter()
            .all(|name| self.walk(&directory.join(name), &format!("{prefix}{name}/")))
    }

    fn report(&self) -> Object {
        let findings = self
            .findings
            .iter()
            .map(|found| {
                let mut item = Object::new();
                item.insert("path", Value::Str(found.path.clone()));
                item.insert("line", Value::int(found.line));
                item.insert("kind", Value::Str(found.kind.to_string()));
                Value::Object(item)
            })
            .collect();
        let skipped = self
            .skipped
            .iter()
            .map(|(path, reason)| {
                let mut item = Object::new();
                item.insert("path", Value::Str(path.clone()));
                item.insert("reason", Value::Str((*reason).to_string()));
                Value::Object(item)
            })
            .collect();
        let status = if self.skipped_count > 0 {
            "incomplete"
        } else {
            "complete"
        };
        let mut report = Object::new();
        report.insert("status", Value::Str(status.to_string()));
        report.insert("findings", Value::Array(findings));
        report.insert("skipped", Value::Array(skipped));
        report.insert("skipped_count", Value::int(self.skipped_count));
        report.insert("scanned_files", Value::Int(self.scanned_files.to_string()));
        report.insert("scanned_bytes", Value::Int(self.scanned_bytes.to_string()));
        report
    }
}

fn is_symlink(path: &Path) -> bool {
    fs::symlink_metadata(path).is_ok_and(|metadata| metadata.file_type().is_symlink())
}

fn ignoring_invalid_utf8(raw: &[u8]) -> Vec<char> {
    raw.utf8_chunks()
        .flat_map(|chunk| chunk.valid().chars())
        .collect()
}

fn files_known_to_git(root: &Path) -> Result<Option<Vec<String>>, String> {
    let arguments = [
        "ls-files",
        "-z",
        "--cached",
        "--others",
        "--exclude-standard",
    ];
    match git::run_git(root, &arguments, None) {
        Err(error) if error.kind == Failure::Spawn => Ok(None),
        Err(error) => Err(error.message),
        Ok(done) if done.code != 0 => Ok(None),
        Ok(done) => Ok(Some(
            done.stdout
                .split(|byte| *byte == 0)
                .filter(|entry| !entry.is_empty())
                .map(|entry| String::from_utf8_lossy(entry).into_owned())
                .collect(),
        )),
    }
}

pub fn exit_code(has_findings: bool, incomplete: bool) -> i32 {
    if has_findings {
        1
    } else if incomplete {
        2
    } else {
        0
    }
}

pub fn scan_report(root: &Path, limits: Limits) -> Result<(Object, i32), String> {
    let mut scan = Scan::new(root.to_path_buf(), limits);
    match files_known_to_git(root)? {
        Some(listed) => scan.examine_listed(&listed),
        None => {
            scan.walk(root, "");
        }
    }
    let code = exit_code(!scan.findings.is_empty(), scan.skipped_count > 0);
    Ok((scan.report(), code))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fixture(name: &str) -> PathBuf {
        let root = std::env::temp_dir().join(format!("secrets-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).unwrap();
        root
    }

    fn status_and_counts(report: &Object) -> (String, String, String) {
        let text = |key: &str| match report.get(key) {
            Some(Value::Str(text)) => text.clone(),
            Some(Value::Int(digits)) => digits.clone(),
            _ => String::new(),
        };
        (text("status"), text("scanned_files"), text("skipped_count"))
    }

    #[test]
    fn the_file_count_limit_stops_the_scan_after_the_last_admitted_file() {
        let root = fixture("count");
        for name in ["a.txt", "b.txt", "c.txt"] {
            fs::write(root.join(name), "x\n").unwrap();
        }
        let limits = Limits { files: 2, ..LIMITS };
        let (report, _) = scan_report(&root, limits).unwrap();
        assert_eq!(
            status_and_counts(&report),
            ("incomplete".into(), "2".into(), "1".into())
        );
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn the_byte_limit_refuses_the_file_that_would_cross_it() {
        let root = fixture("bytes");
        fs::write(root.join("a.txt"), "12345\n").unwrap();
        fs::write(root.join("b.txt"), "12345\n").unwrap();
        let limits = Limits {
            bytes: 11,
            ..LIMITS
        };
        let (report, _) = scan_report(&root, limits).unwrap();
        assert_eq!(
            status_and_counts(&report),
            ("incomplete".into(), "1".into(), "1".into())
        );
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn findings_outrank_an_incomplete_scan_in_the_exit_code() {
        assert_eq!(exit_code(false, false), 0);
        assert_eq!(exit_code(false, true), 2);
        assert_eq!(exit_code(true, false), 1);
        assert_eq!(exit_code(true, true), 1);
    }

    #[test]
    fn the_report_names_at_most_a_thousand_skipped_files_but_counts_all_of_them() {
        let mut scan = Scan::new(PathBuf::from("."), LIMITS);
        for number in 0..=MAX_REPORTED_SKIPS {
            scan.skip(&format!("f{number}"), "symlink");
        }
        assert_eq!(scan.skipped.len(), MAX_REPORTED_SKIPS);
        assert_eq!(scan.skipped_count, MAX_REPORTED_SKIPS + 1);
        assert_eq!(scan.skipped[MAX_REPORTED_SKIPS - 1].0, "f999");
    }

    #[test]
    fn a_file_exactly_at_the_size_limit_is_scanned_and_one_byte_over_is_refused() {
        let root = fixture("size");
        fs::write(root.join("at.txt"), "123456\n").unwrap();
        fs::write(root.join("over.txt"), "1234567\n").unwrap();
        let limits = Limits {
            file_bytes: 7,
            ..LIMITS
        };
        let (report, _) = scan_report(&root, limits).unwrap();
        assert_eq!(
            status_and_counts(&report),
            ("incomplete".into(), "1".into(), "1".into())
        );
        let _ = fs::remove_dir_all(&root);
    }
}

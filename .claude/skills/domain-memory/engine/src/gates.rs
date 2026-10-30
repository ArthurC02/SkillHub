use std::collections::HashSet;
use std::path::{Path, PathBuf};

use crate::git_governance::hooks_dir;
use crate::json::{self, Object, Value};
use crate::listing::{EXCLUDED_DIRECTORIES, ignored_among, listing_of};
use crate::py;
use crate::store::shown_path;

const FORMAT: &str = "domain-memory-quality-gates/v1";
const KINDS: [&str; 6] = ["lint", "types", "architecture", "tests", "hooks", "ci"];
const STANDARD_KINDS: [&str; 3] = ["lint", "types", "architecture"];
const GO_MODULE: &str = "go.mod";
const FIXTURE_DIRECTORIES: [&str; 3] = ["fixtures", "__fixtures__", "testdata"];
const TASKFILE_NAMES: [&str; 4] = [
    "Taskfile.yml",
    "Taskfile.yaml",
    "taskfile.yml",
    "taskfile.yaml",
];

const MARKER_FILES: [(&str, &str, &str); 20] = [
    (".golangci.yml", "lint", "golangci-lint"),
    (".golangci.yaml", "lint", "golangci-lint"),
    ("ruff.toml", "lint", "ruff"),
    (".ruff.toml", "lint", "ruff"),
    (".flake8", "lint", "flake8"),
    (".pylintrc", "lint", "pylint"),
    (".oxlintrc.json", "lint", "oxlint"),
    ("biome.json", "lint", "biome"),
    (".rubocop.yml", "lint", "rubocop"),
    ("clippy.toml", "lint", "clippy"),
    ("checkstyle.xml", "lint", "checkstyle"),
    (".shellcheckrc", "lint", "shellcheck"),
    ("tsconfig.json", "types", "typescript"),
    ("mypy.ini", "types", "mypy"),
    ("pyrightconfig.json", "types", "pyright"),
    (".importlinter", "architecture", "import-linter"),
    (".go-arch-lint.yml", "architecture", "go-arch-lint"),
    ("pytest.ini", "tests", "pytest"),
    (".pre-commit-config.yaml", "hooks", "pre-commit"),
    ("lefthook.yml", "hooks", "lefthook"),
];
const MARKER_PREFIXES: [(&str, &str, &str); 6] = [
    (".eslintrc", "lint", "eslint"),
    ("eslint.config.", "lint", "eslint"),
    (".dependency-cruiser.", "architecture", "dependency-cruiser"),
    ("jest.config.", "tests", "jest"),
    ("vitest.config.", "tests", "vitest"),
    ("playwright.config.", "tests", "playwright"),
];
const PYPROJECT_TOOLS: [(&str, &str); 7] = [
    ("ruff", "lint"),
    ("flake8", "lint"),
    ("pylint", "lint"),
    ("mypy", "types"),
    ("pyright", "types"),
    ("importlinter", "architecture"),
    ("pytest", "tests"),
];
const PACKAGE_SCRIPTS: [(&str, &str); 3] =
    [("lint", "lint"), ("typecheck", "types"), ("test", "tests")];
const TASK_PREFIXES: [(&str, &str); 3] =
    [("lint", "lint"), ("typecheck", "types"), ("test", "tests")];
const GIT_HOOK_NAMES: [&str; 19] = [
    "applypatch-msg",
    "pre-applypatch",
    "post-applypatch",
    "pre-commit",
    "pre-merge-commit",
    "prepare-commit-msg",
    "commit-msg",
    "post-commit",
    "pre-rebase",
    "post-checkout",
    "post-merge",
    "pre-push",
    "pre-receive",
    "post-receive",
    "post-update",
    "push-to-checkout",
    "pre-auto-gc",
    "post-rewrite",
    "reference-transaction",
];
const CI_MARKERS: [(&str, &str); 6] = [
    (".github/workflows", "github-actions"),
    (".gitlab-ci.yml", "gitlab-ci"),
    ("Jenkinsfile", "jenkins"),
    (".circleci/config.yml", "circleci"),
    (".buildkite/pipeline.yml", "buildkite"),
    ("azure-pipelines.yml", "azure-pipelines"),
];

struct Gate {
    kind: &'static str,
    tool: String,
    path: String,
    enabled: Option<bool>,
}

impl Gate {
    fn new(kind: &'static str, tool: impl Into<String>, path: &str) -> Self {
        Self {
            kind,
            tool: tool.into(),
            path: path.to_string(),
            enabled: None,
        }
    }

    fn shown(&self) -> Value {
        let mut gate = Object::new();
        gate.insert("kind", Value::Str(self.kind.to_string()));
        gate.insert("tool", Value::Str(self.tool.clone()));
        gate.insert("path", Value::Str(self.path.clone()));
        if let Some(enabled) = self.enabled {
            gate.insert("enabled", Value::Bool(enabled));
        }
        Value::Object(gate)
    }
}

fn file_name(relative: &str) -> &str {
    relative.rsplit_once('/').map_or(relative, |(_, name)| name)
}

fn is_test_file(name: &str) -> bool {
    name.ends_with("_test.go")
        || name.ends_with("_test.py")
        || name.contains(".test.")
        || name.contains(".spec.")
        || (name.starts_with("test_") && name.ends_with(".py"))
}

fn in_fixture(entry: &str) -> bool {
    entry
        .split('/')
        .any(|part| FIXTURE_DIRECTORIES.contains(&part))
}

fn marker_for(name: &str) -> Option<(&'static str, &'static str)> {
    if let Some((_, kind, tool)) = MARKER_FILES.iter().find(|(file, _, _)| *file == name) {
        return Some((kind, tool));
    }
    MARKER_PREFIXES
        .iter()
        .find(|(prefix, _, _)| name.starts_with(prefix))
        .map(|(_, kind, tool)| (*kind, *tool))
}

fn has_reader(name: &str) -> bool {
    matches!(
        name,
        "pyproject.toml" | "package.json" | ".golangci.yml" | ".golangci.yaml"
    ) || TASKFILE_NAMES.contains(&name)
}

fn sorted_where(files: &[String], keep: impl Fn(&str) -> bool) -> Vec<String> {
    let mut found: Vec<String> = files.iter().filter(|entry| keep(entry)).cloned().collect();
    found.sort();
    found
}

fn candidate_files(files: &[String]) -> Vec<String> {
    sorted_where(files, |entry| {
        let name = file_name(entry);
        (marker_for(name).is_some() || has_reader(name) || name == GO_MODULE) && !in_fixture(entry)
    })
}

fn read_text(path: &Path) -> Result<Option<String>, String> {
    let bytes = std::fs::read(path).map_err(|error| py::open_error(&error, path))?;
    Ok(py::utf8_text(&bytes).ok())
}

fn is_table_char(character: char) -> bool {
    character.is_ascii_alphanumeric() || character == '_' || character == '-'
}

fn pyproject_gates(path: &Path, shown: &str) -> Result<Vec<Gate>, String> {
    let Some(text) = read_text(path)? else {
        return Ok(Vec::new());
    };
    let tables: HashSet<&str> = text
        .split('\n')
        .filter_map(|line| line.strip_prefix("[tool."))
        .map(|rest| {
            let end = rest.find(|c: char| !is_table_char(c)).unwrap_or(rest.len());
            &rest[..end]
        })
        .collect();
    Ok(PYPROJECT_TOOLS
        .iter()
        .filter(|(tool, _)| tables.contains(tool))
        .map(|(tool, kind)| Gate::new(kind, *tool, shown))
        .collect())
}

fn package_gates(path: &Path, shown: &str) -> Result<Vec<Gate>, String> {
    let Some(text) = read_text(path)? else {
        return Ok(Vec::new());
    };
    let Ok(Value::Object(document)) = json::parse_keeping_last_duplicate(&text) else {
        return Ok(Vec::new());
    };
    let Some(Value::Object(scripts)) = document.get("scripts") else {
        return Ok(Vec::new());
    };
    Ok(PACKAGE_SCRIPTS
        .iter()
        .filter(|(name, _)| scripts.get(name).is_some())
        .map(|(name, kind)| Gate::new(kind, format!("script:{name}"), shown))
        .collect())
}

fn tasks_section(text: &str) -> Option<&str> {
    let mut offset = 0;
    for line in text.split('\n') {
        let end = offset + line.len();
        let declares = line
            .strip_prefix("tasks:")
            .is_some_and(|rest| rest.chars().all(|c| c == ' ' || c == '\t'));
        if declares {
            return Some(&text[end..section_end(text, end)]);
        }
        offset = end + 1;
    }
    None
}

fn section_end(text: &str, from: usize) -> usize {
    let mut search = from;
    while let Some(newline) = text[search..].find('\n') {
        let start = search + newline + 1;
        if text[start..]
            .chars()
            .next()
            .is_some_and(|c| !py::is_space(c))
        {
            return start;
        }
        search = start;
    }
    text.len()
}

fn is_task_char(character: char) -> bool {
    character.is_ascii_alphanumeric() || matches!(character, '_' | '.' | ':' | '-')
}

fn task_name(line: &str) -> Option<&str> {
    let rest = line.strip_prefix("  ")?;
    let rest = rest.strip_prefix(['\'', '"']).unwrap_or(rest);
    let run_length = rest.find(|c: char| !is_task_char(c)).unwrap_or(rest.len());
    let closes_quote = rest[run_length..]
        .strip_prefix(['\'', '"'])
        .is_some_and(|tail| tail.starts_with(':'));
    if run_length > 0 && closes_quote {
        return Some(&rest[..run_length]);
    }
    let run = &rest.as_bytes()[..run_length];
    (1..run_length)
        .rev()
        .find(|end| run[*end] == b':')
        .map(|end| &rest[..end])
}

fn taskfile_gates(path: &Path, shown: &str) -> Result<Vec<Gate>, String> {
    let Some(text) = read_text(path)? else {
        return Ok(Vec::new());
    };
    let Some(section) = tasks_section(&text) else {
        return Ok(Vec::new());
    };
    let mut gates = Vec::new();
    for name in section.split('\n').filter_map(task_name) {
        for (prefix, kind) in TASK_PREFIXES {
            if name.starts_with(prefix) {
                gates.push(Gate::new(kind, format!("task:{name}"), shown));
            }
        }
    }
    Ok(gates)
}

fn declares_depguard(text: &str) -> bool {
    text.match_indices("depguard").any(|(at, word)| {
        let before = &text[..at];
        let trimmed = before.trim_end_matches(py::is_space);
        let anchored = trimmed.is_empty() || before[trimmed.len()..].contains('\n');
        anchored
            && text[at + word.len()..]
                .trim_start_matches(py::is_space)
                .starts_with(':')
    })
}

fn golangci_gates(path: &Path, shown: &str) -> Result<Vec<Gate>, String> {
    let Some(text) = read_text(path)? else {
        return Ok(Vec::new());
    };
    Ok(if declares_depguard(&text) {
        vec![Gate::new("architecture", "depguard", shown)]
    } else {
        Vec::new()
    })
}

fn configured_gates(root: &Path, candidates: &[&String]) -> Result<Vec<Gate>, String> {
    let mut gates = Vec::new();
    for shown in candidates {
        let name = file_name(shown);
        if let Some((kind, tool)) = marker_for(name) {
            gates.push(Gate::new(kind, tool, shown));
        }
        let path = root.join(shown.as_str());
        match name {
            "pyproject.toml" => gates.extend(pyproject_gates(&path, shown)?),
            "package.json" => gates.extend(package_gates(&path, shown)?),
            ".golangci.yml" | ".golangci.yaml" => gates.extend(golangci_gates(&path, shown)?),
            _ if TASKFILE_NAMES.contains(&name) => gates.extend(taskfile_gates(&path, shown)?),
            _ => {}
        }
    }
    Ok(gates)
}

fn inside(path: &str, directory: &str) -> bool {
    directory == "." || path.starts_with(&format!("{directory}/"))
}

fn go_test_gates(candidates: &[&String], tests: &[&String]) -> Vec<Gate> {
    let tests: Vec<&str> = tests
        .iter()
        .filter(|entry| entry.ends_with("_test.go") && !in_fixture(entry))
        .map(|entry| entry.as_str())
        .collect();
    candidates
        .iter()
        .filter(|entry| file_name(entry) == GO_MODULE)
        .filter(|module| {
            let directory = module.rsplit_once('/').map_or(".", |(parent, _)| parent);
            tests.iter().any(|test| inside(test, directory))
        })
        .map(|module| Gate::new("tests", "go-test", module))
        .collect()
}

fn installed_hooks(root: &Path) -> Result<Vec<Gate>, String> {
    let hooks = hooks_dir(root)?;
    if !hooks.is_dir() {
        return Ok(Vec::new());
    }
    let mut entries: Vec<PathBuf> = std::fs::read_dir(&hooks)
        .map_err(|error| py::os_error(&error, &hooks))?
        .map(|entry| entry.map(|found| found.path()))
        .collect::<Result<_, _>>()
        .map_err(|error| py::os_error(&error, &hooks))?;
    entries.sort_by(|left, right| py::compare_paths(left, right));
    Ok(entries
        .iter()
        .filter_map(|entry| {
            let name = entry.file_name()?.to_string_lossy().into_owned();
            (entry.is_file() && !name.contains('.')).then(|| {
                Gate::new(
                    "hooks",
                    format!("git:{name}"),
                    &shown_path(entry, Some(root)),
                )
            })
        })
        .collect())
}

fn shipped_hooks(files: &[String], installed: &[Gate]) -> Vec<Gate> {
    let enabled: HashSet<&str> = installed.iter().map(|gate| gate.path.as_str()).collect();
    sorted_where(files, |path| {
        GIT_HOOK_NAMES.contains(&file_name(path)) && !enabled.contains(path)
    })
    .iter()
    .map(|path| Gate {
        enabled: Some(false),
        ..Gate::new("hooks", format!("git:{}", file_name(path)), path)
    })
    .collect()
}

fn ci_gates(root: &Path) -> Vec<Gate> {
    let mut found: Vec<&(&str, &str)> = CI_MARKERS
        .iter()
        .filter(|(path, _)| root.join(path).exists())
        .collect();
    found.sort_by_key(|(_, tool)| *tool);
    found
        .iter()
        .map(|(path, tool)| Gate::new("ci", *tool, path))
        .collect()
}

fn words(kinds: Vec<&str>) -> Value {
    Value::Array(
        kinds
            .into_iter()
            .map(|kind| Value::Str(kind.to_string()))
            .collect(),
    )
}

pub fn quality_gates(given: &str) -> Result<Value, String> {
    let root = py::resolve(given);
    if !root.is_dir() {
        return Err(format!(
            "repository root is not a directory: {}",
            py::PurePath::new(given).native()
        ));
    }
    let excluded: HashSet<String> = EXCLUDED_DIRECTORIES
        .iter()
        .map(|name| (*name).to_string())
        .collect();
    let files = listing_of(&root, &excluded)?;
    let candidates = candidate_files(&files.entries);
    let tests = sorted_where(&files.entries, |entry| is_test_file(file_name(entry)));
    let asked: Vec<String> = candidates.iter().chain(&tests).cloned().collect();
    let ignored = ignored_among(&root, &files, &asked)?;
    let kept_candidates: Vec<&String> = candidates
        .iter()
        .filter(|entry| !ignored.contains(*entry))
        .collect();
    let kept_tests: Vec<&String> = tests
        .iter()
        .filter(|entry| !ignored.contains(*entry))
        .collect();
    let mut gates = configured_gates(&root, &kept_candidates)?;
    gates.extend(go_test_gates(&kept_candidates, &kept_tests));
    let installed = installed_hooks(&root)?;
    let shipped = shipped_hooks(&files.entries, &installed);
    gates.extend(installed);
    gates.extend(shipped);
    gates.extend(ci_gates(&root));
    let mut present: Vec<&str> = gates.iter().map(|gate| gate.kind).collect();
    present.sort_unstable();
    present.dedup();
    let missing = KINDS
        .iter()
        .copied()
        .filter(|kind| !present.contains(kind))
        .collect();
    let standard = STANDARD_KINDS.iter().any(|kind| present.contains(kind));
    let mut report = Object::new();
    report.insert("format", Value::Str(FORMAT.to_string()));
    report.insert(
        "gates",
        Value::Array(gates.iter().map(Gate::shown).collect()),
    );
    report.insert("kinds_present", words(present));
    report.insert("kinds_missing", words(missing));
    let counted = kept_tests.iter().filter(|entry| !in_fixture(entry)).count();
    report.insert("test_files", Value::int(counted));
    let standard = if standard { "configured" } else { "none" };
    report.insert("standard", Value::Str(standard.to_string()));
    Ok(Value::Object(report))
}

#[cfg(test)]
mod tests {
    use super::{Gate, shipped_hooks};

    #[test]
    fn shipped_hooks_are_ordered_by_path_and_exclude_installed_hooks() {
        let installed = vec![Gate::new("hooks", "git:pre-commit", "hooks/pre-commit")];
        for paths in [
            [
                "tools/commit-msg",
                "hooks/pre-push",
                "hooks/pre-commit",
                "notes.txt",
            ],
            [
                "notes.txt",
                "hooks/pre-commit",
                "hooks/pre-push",
                "tools/commit-msg",
            ],
        ] {
            let files = paths.map(String::from);
            let actual: Vec<_> = shipped_hooks(&files, &installed)
                .into_iter()
                .map(|gate| (gate.kind, gate.tool, gate.path, gate.enabled))
                .collect();
            assert_eq!(
                actual,
                vec![
                    (
                        "hooks",
                        "git:pre-push".into(),
                        "hooks/pre-push".into(),
                        Some(false)
                    ),
                    (
                        "hooks",
                        "git:commit-msg".into(),
                        "tools/commit-msg".into(),
                        Some(false)
                    ),
                ]
            );
        }
    }
}

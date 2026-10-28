use std::path::{Path, PathBuf};

use crate::coverage::SourceKinds;
use crate::digits::decimal_value;
use crate::evidence::{MAX_EVIDENCE_BYTES, WIDEN_TO_UNIQUE, rows};
use crate::files::load_json;
use crate::json::{Object, Value};
use crate::listing::{default_excluded, listing_of};
use crate::py;
use crate::pyregex::{self, Match, Regex, escape};
use crate::pyvalue::splitlines;
use crate::sha256::hex_digest;

const INDENT: &str = r"^(?P<indent>[ \t]*)";
const GO_START: &str = r"func(?:\s*\([^)]*\))?\s+{name}\s*(?P<open>[(\[])";
const PYTHON_START: &str = r"(?:async\s+)?def\s+{name}\s*[(\[]";
const SCRIPT_START: &str = r"(?:export\s+)?(?:default\s+)?(?:(?:async\s+)?function\s*\*?\s*{name}\s*(?P<open>[(<])|(?:const|let|var)\s+{name}\s*(?::[^=]*?)?(?P<arrow>=)(?!=)|(?:(?:public|private|protected|static|async|readonly|override)\s+)*{name}\s*(?P<method>[(<]))";
const JAVA_START: &str = r"(?!return\b|new\b|throw\b|else\b)(?:(?:public|private|protected|static|final|abstract|synchronized|native|default)\s+)*(?:<[^>]+>\s+)?(?:[\w.$<>\[\],?]+\s+)?{name}\s*(?P<open>\()";
const KOTLIN_START: &str = r"(?:(?:public|private|protected|internal|override|open|abstract|final|suspend|inline|infix|operator|tailrec|external|actual|expect)\s+)*fun\s+(?:<[^>]+>\s+)?(?:[\w.<>?]+\.)?{name}\s*(?P<open>[(<])";
const RUST_START: &str = r#"(?:pub(?:\([^)]*\))?\s+)?(?:(?:const|async|unsafe|extern\s+"[^"]*")\s+)*fn\s+{name}\s*(?P<open>[(<])"#;
const LANGUAGES: &[(&str, &str, &str)] = &[
    (".go", "Go", GO_START),
    (".py", "Python", PYTHON_START),
    (".ts", "TypeScript", SCRIPT_START),
    (".tsx", "TypeScript", SCRIPT_START),
    (".js", "JavaScript", SCRIPT_START),
    (".jsx", "JavaScript", SCRIPT_START),
    (".mjs", "JavaScript", SCRIPT_START),
    (".java", "Java", JAVA_START),
    (".kt", "Kotlin", KOTLIN_START),
    (".rs", "Rust", RUST_START),
];
const LANGUAGE_NAMES: &str = "Go, Python, TypeScript, JavaScript, Java, Kotlin, Rust";
const QUOTES: &str = "'\"`";
const OPENERS: &str = "([{";
const CLOSERS: &str = ")]}";
const REGEX_AFTER_CHARACTERS: &str = "(,=:[!&|?{};+-<>\n";
const REGEX_AFTER_WORDS: &[&str] = &[
    "return",
    "typeof",
    "case",
    "in",
    "of",
    "void",
    "delete",
    "throw",
    "await",
    "yield",
    "instanceof",
];
pub const OUTSIDE_SOURCES_CONSEQUENCE: &str = "A candidate may rest on it, but ship-package refuses to review a record whose evidence rests \
     outside the confirmed sources; cite a source the corpus covers, or have the developer confirm this path \
     before the record is reviewed.";

#[derive(Clone, Copy, PartialEq)]
enum Kind {
    Block,
    Expression,
    Line,
}

type Body = (Kind, usize);

pub struct Cited {
    pub reference: Object,
    pub notes: Vec<String>,
}

pub fn cite(
    repo_root: &Path,
    relative: &str,
    lines: Option<&str>,
    function: Option<&str>,
    registry_root: Option<&Path>,
    show: bool,
) -> Result<Cited, String> {
    let root = py::resolve_path(repo_root);
    let (start, end) = match (lines, function) {
        (Some(lines), None) => line_range(lines)?,
        (None, Some(function)) => {
            let (start, end) = function_span(&root, relative, function)?;
            (start.to_string(), end.to_string())
        }
        _ => return Err("cite takes --lines START-END or --function NAME".to_string()),
    };
    let mut reference = citation(&root, relative, &start, &end)?;
    if let Some(registry_root) = registry_root {
        let source_map_path = py::resolve_path(registry_root).join("source-map.json");
        let source_map = if source_map_path.is_file() {
            load_json(&source_map_path)?
        } else {
            Object::new()
        };
        if let Value::Object(classified) =
            SourceKinds::of(&source_map).classify(&Value::Object(reference.clone()))
        {
            reference = classified;
        }
    }
    let path_text = reference
        .get("path")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string();
    let (start, end) = (to_count(&start), to_count(&end));
    let mut notes = Vec::new();
    if show {
        let content = std::fs::read(root.join(&path_text)).unwrap_or_default();
        let text = String::from_utf8_lossy(&content);
        let shown = splitlines(&text);
        let row = |number: usize| shown.get(number - 1).copied().unwrap_or_default();
        notes.push(format!("{path_text}:{start}: {}", row(start)));
        if end - start > 1 {
            notes.push(format!(
                "{path_text}: ... {} lines not shown",
                end - start - 1
            ));
        }
        if end > start {
            notes.push(format!("{path_text}:{end}: {}", row(end)));
        }
    }
    notes.extend(repeated_note(&root, &reference));
    if reference.get("source_kind").and_then(Value::as_str) == Some("unclassified") {
        notes.push(format!(
            "NOTE: {path_text} is outside the confirmed sources. {OUTSIDE_SOURCES_CONSEQUENCE}"
        ));
    }
    Ok(Cited { reference, notes })
}

pub fn repeated_note(root: &Path, cited: &Object) -> Option<String> {
    let text = |key: &str| {
        cited
            .get(key)
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string()
    };
    let (path_text, excerpt) = (text("path"), text("excerpt_sha256"));
    let lines = cited.get("lines").and_then(Value::as_object)?;
    let bound = |key: &str| match lines.get(key) {
        Some(Value::Int(digits)) => to_count(digits),
        _ => 0,
    };
    let (start, end) = (bound("start"), bound("end"));
    let others: Vec<String> = excerpt_starts(root, &path_text, end - start + 1, &excerpt)
        .into_iter()
        .filter(|line| *line != start)
        .map(|line| line.to_string())
        .collect();
    if others.is_empty() {
        return None;
    }
    Some(format!(
        "NOTE: the text of {path_text}:{start}-{end} also starts at line {}, so a later edit cannot tell the copies apart. {WIDEN_TO_UNIQUE}",
        others.join(", ")
    ))
}

fn to_count(digits: &str) -> usize {
    digits.parse().unwrap_or(usize::MAX)
}

pub fn whole_number(text: &str) -> Option<String> {
    if text.is_empty() {
        return None;
    }
    let digits: Option<String> = text
        .chars()
        .map(|c| decimal_value(c).and_then(|value| char::from_digit(value, 10)))
        .collect();
    let digits = digits?;
    let trimmed = digits.trim_start_matches('0');
    Some(if trimmed.is_empty() {
        "0".to_string()
    } else {
        trimmed.to_string()
    })
}

pub fn line_range(text: &str) -> Result<(String, String), String> {
    let (start, end) = text.split_once('-').unwrap_or((text, ""));
    let refused = || format!("lines must be START or START-END, got {text}");
    let start_number = whole_number(start).ok_or_else(refused)?;
    let end_number = if end.is_empty() {
        start_number.clone()
    } else {
        whole_number(end).ok_or_else(refused)?
    };
    Ok((start_number, end_number))
}

fn numerically_less(left: &str, right: &str) -> bool {
    (left.len(), left) < (right.len(), right)
}

fn cited_file(root: &Path, relative: &str) -> Result<PathBuf, String> {
    let path = py::resolve_path(&root.join(relative));
    if !path.starts_with(root) {
        return Err(format!("evidence path escapes the repository: {relative}"));
    }
    if !path.is_file() {
        return Err(format!("no such file to cite: {relative}"));
    }
    Ok(path)
}

fn sha256_of(data: &[u8]) -> String {
    format!("sha256:{}", hex_digest(data))
}

pub fn citation(root: &Path, relative: &str, start: &str, end: &str) -> Result<Object, String> {
    let path = cited_file(root, relative)?;
    if path.metadata().map(|meta| meta.len()).unwrap_or(0) > MAX_EVIDENCE_BYTES {
        return Err(format!(
            "evidence source exceeds {MAX_EVIDENCE_BYTES} bytes: {relative}"
        ));
    }
    if numerically_less(start, "1") || numerically_less(end, start) {
        return Err(format!(
            "a citation needs 1 <= start <= end, got start={start} end={end}"
        ));
    }
    let content = std::fs::read(&path).map_err(|error| py::os_error(&error, &path))?;
    let rows = rows(&content);
    if numerically_less(&rows.len().to_string(), end) {
        return Err(format!(
            "{relative} has {} lines; the citation asks for line {end}",
            rows.len()
        ));
    }
    let (first, last) = (to_count(start), to_count(end));
    let mut lines = Object::new();
    lines.insert("start", Value::Int(start.to_string()));
    lines.insert("end", Value::Int(end.to_string()));
    let mut reference = Object::new();
    reference.insert("path", Value::Str(py::relative_to(&path, root)?));
    reference.insert("lines", Value::Object(lines));
    reference.insert("content_sha256", Value::Str(sha256_of(&content)));
    reference.insert(
        "excerpt_sha256",
        Value::Str(sha256_of(rows[first - 1..last].concat().as_bytes())),
    );
    Ok(reference)
}

pub fn excerpt_starts(root: &Path, relative: &str, length: usize, excerpt: &str) -> Vec<usize> {
    let path = py::resolve_path(&root.join(relative));
    if !path.starts_with(root)
        || !path.is_file()
        || path.metadata().map(|meta| meta.len()).unwrap_or(0) > MAX_EVIDENCE_BYTES
    {
        return Vec::new();
    }
    let rows = rows(&std::fs::read(&path).unwrap_or_default());
    if length == 0 || length > rows.len() {
        return Vec::new();
    }
    (1..=rows.len() + 1 - length)
        .filter(|start| {
            sha256_of(rows[start - 1..start - 1 + length].concat().as_bytes()) == excerpt
        })
        .collect()
}

fn suffix(relative: &str) -> &str {
    let name = relative
        .rsplit(|c| c == '/' || (cfg!(windows) && c == '\\'))
        .next()
        .unwrap_or(relative);
    match name.rfind('.') {
        Some(at) if at > 0 && at + 1 < name.len() => &name[at..],
        _ => "",
    }
}

fn language(relative: &str) -> Result<(&'static str, &'static str), String> {
    let suffix = suffix(relative);
    LANGUAGES
        .iter()
        .find(|(known, _, _)| *known == suffix)
        .map(|(_, language, start)| (*language, *start))
        .ok_or_else(|| {
            let shown = if suffix.is_empty() {
                "a file without a suffix".to_string()
            } else {
                format!("a {suffix} file")
            };
            format!(
                "{relative}: --function reads {LANGUAGE_NAMES} files; cite {shown} with --lines"
            )
        })
}

pub fn function_span(root: &Path, relative: &str, name: &str) -> Result<(usize, usize), String> {
    let (language, start_form) = language(relative)?;
    let path = cited_file(root, relative)?;
    let rows = rows(&std::fs::read(&path).map_err(|error| py::os_error(&error, &path))?);
    let pattern = pyregex::compile(&format!(
        "{INDENT}{}",
        start_form.replace("{name}", &escape(name))
    ))?;
    let starts = definitions(&rows, language, &pattern);
    if starts.is_empty() {
        return Err(format!(
            "{relative} defines no function named {name} in a form --function reads{}; a definition of another form is cited with --lines",
            defined_elsewhere(root, relative, language, &pattern)?
        ));
    }
    if starts.len() != 1 {
        return Err(format!(
            "{relative} defines {name} {} times; cite the one you mean with --lines",
            starts.len()
        ));
    }
    let (start, found, body) = &starts[0];
    let end = if language == "Python" {
        let indent = found.named("indent").map_or(0, |(from, to)| to - from);
        python_end(&rows, *start, indent)
    } else {
        body.and_then(|body| Source::new(&rows, language).end(body))
    };
    let end = end.ok_or_else(|| {
        format!("{relative}: the body of {name} starting on line {start} does not end; cite it with --lines")
    })?;
    Ok((*start, end))
}

fn definitions(
    rows: &[String],
    language: &str,
    pattern: &Regex,
) -> Vec<(usize, Match, Option<Body>)> {
    let source = Source::new(rows, language);
    let mut found_all = Vec::new();
    for (index, row) in rows.iter().enumerate() {
        let number = index + 1;
        let chars: Vec<char> = row.chars().collect();
        let Some(found) = pattern.match_at(&chars, 0) else {
            continue;
        };
        if language == "Python" {
            found_all.push((number, found, None));
            continue;
        }
        if let Some(body) = source.definition(language, number, &found) {
            found_all.push((number, found, Some(body)));
        }
    }
    found_all
}

fn defined_elsewhere(
    root: &Path,
    relative: &str,
    language: &str,
    pattern: &Regex,
) -> Result<String, String> {
    let suffix = suffix(relative);
    let mut found = Vec::new();
    for path in listing_of(root, &default_excluded())?.entries {
        if path == relative || !path.ends_with(suffix) {
            continue;
        }
        let full = root.join(&path);
        let data = std::fs::read(&full).map_err(|error| py::open_error(&error, &full))?;
        found.extend(
            definitions(&rows(&data), language, pattern)
                .into_iter()
                .map(|(number, _, _)| format!("{path}:{number}")),
        );
    }
    found.sort();
    Ok(match found.as_slice() {
        [] => String::new(),
        [only] => format!("; it is defined in {only}"),
        many => format!("; it is defined in several files: {}", many.join(", ")),
    })
}

fn python_end(rows: &[String], start: usize, indent: usize) -> Option<usize> {
    let signature_end = (start..=rows.len()).find(|number| {
        rows[number - 1]
            .trim_end_matches(py::is_space)
            .ends_with(':')
    })?;
    let mut end = signature_end;
    for number in signature_end + 1..=rows.len() {
        let row = &rows[number - 1];
        if row.trim_matches(py::is_space).is_empty() {
            continue;
        }
        let leading = row.chars().count() - row.trim_start_matches(py::is_space).chars().count();
        if leading <= indent {
            break;
        }
        end = number;
    }
    Some(end)
}

fn is_name_char(c: char) -> bool {
    c.is_alphanumeric() || c == '_' || c == '$'
}

struct Source {
    text: Vec<char>,
    lifetimes: bool,
    regex_literals: bool,
    offsets: Vec<usize>,
}

impl Source {
    fn new(rows: &[String], language: &str) -> Self {
        let mut offsets = vec![0];
        let mut text = Vec::new();
        for row in rows {
            text.extend(row.chars());
            offsets.push(text.len());
        }
        Self {
            text,
            lifetimes: language == "Rust",
            regex_literals: language == "TypeScript" || language == "JavaScript",
            offsets,
        }
    }

    fn at(&self, index: usize) -> Option<char> {
        self.text.get(index).copied()
    }

    fn starts_with(&self, wanted: &str, index: usize) -> bool {
        wanted
            .chars()
            .enumerate()
            .all(|(offset, c)| self.at(index + offset) == Some(c))
    }

    fn find(&self, wanted: &str, from: usize) -> Option<usize> {
        (from..self.text.len()).find(|index| self.starts_with(wanted, *index))
    }

    fn line_of(&self, index: usize) -> usize {
        self.offsets.partition_point(|offset| *offset <= index)
    }

    fn word_at(&self, index: usize) -> String {
        let mut end = index;
        while self.at(end).is_some_and(is_name_char) {
            end += 1;
        }
        self.text[index.min(self.text.len())..end.min(self.text.len())]
            .iter()
            .collect()
    }

    fn skip_string(&self, index: usize) -> usize {
        let quote = self.text[index];
        if quote == '\'' && self.lifetimes && !self.starts_with("'", index + 2) {
            return index + 1;
        }
        let mut position = index + 1;
        while position < self.text.len() {
            let c = self.text[position];
            if c == '\\' {
                position += 2;
                continue;
            }
            if c == quote {
                return position + 1;
            }
            if quote == '`' && self.starts_with("${", position) {
                match self.skip_balanced(position + 2) {
                    None => return self.text.len(),
                    Some(closed) => {
                        position = closed;
                        continue;
                    }
                }
            }
            if quote != '`' && c == '\n' {
                return position;
            }
            position += 1;
        }
        self.text.len()
    }

    fn significant(&self, mut index: usize) -> usize {
        while index < self.text.len() {
            let c = self.text[index];
            if py::is_space(c) {
                index += 1;
            } else if self.starts_with("//", index) {
                index = self.find("\n", index).unwrap_or(self.text.len());
            } else if self.starts_with("/*", index) {
                index = self
                    .find("*/", index + 2)
                    .map_or(self.text.len(), |closed| closed + 2);
            } else if QUOTES.contains(c) {
                index = self.skip_string(index);
            } else if c == '/' && self.regex_starts(index) {
                index = self.skip_regex(index);
            } else {
                return index;
            }
        }
        index
    }

    fn regex_starts(&self, index: usize) -> bool {
        if !self.regex_literals {
            return false;
        }
        let mut back = index as isize - 1;
        while back >= 0 && " \t".contains(self.text[back as usize]) {
            back -= 1;
        }
        if back < 0 || REGEX_AFTER_CHARACTERS.contains(self.text[back as usize]) {
            return true;
        }
        let mut start = back;
        while start >= 0 && is_name_char(self.text[start as usize]) {
            start -= 1;
        }
        let word: String = self.text[(start + 1) as usize..=back as usize]
            .iter()
            .collect();
        REGEX_AFTER_WORDS.contains(&word.as_str())
    }

    fn skip_regex(&self, index: usize) -> usize {
        let mut position = index + 1;
        let mut in_class = false;
        while position < self.text.len() {
            let c = self.text[position];
            if c == '\\' {
                position += 2;
                continue;
            }
            if c == '\n' {
                return position;
            }
            if in_class {
                in_class = c != ']';
            } else if c == '[' {
                in_class = true;
            } else if c == '/' {
                position += 1;
                while self.at(position).is_some_and(char::is_alphabetic) {
                    position += 1;
                }
                return position;
            }
            position += 1;
        }
        position
    }

    fn skip_balanced(&self, mut index: usize) -> Option<usize> {
        let mut depth = 1;
        loop {
            index = self.significant(index);
            let c = self.at(index)?;
            if OPENERS.contains(c) {
                depth += 1;
            } else if CLOSERS.contains(c) {
                depth -= 1;
                if depth == 0 {
                    return Some(index + 1);
                }
            }
            index += 1;
        }
    }

    fn skip_type_parameters(&self, index: usize) -> Option<usize> {
        let closed = if self.text[index] == '[' {
            self.skip_balanced(index + 1)
        } else {
            let mut depth = 0i64;
            let mut closed = None;
            for position in index..self.text.len() {
                depth += match self.text[position] {
                    '<' => 1,
                    '>' => -1,
                    _ => 0,
                };
                if depth == 0 {
                    closed = Some(position + 1);
                    break;
                }
            }
            closed
        }?;
        let opener = self.significant(closed);
        (self.at(opener) == Some('(')).then_some(opener)
    }

    fn definition(&self, language: &str, number: usize, found: &Match) -> Option<Body> {
        let start = self.offsets[number - 1];
        if found.named("arrow").is_some_and(|(from, to)| to > from) {
            return self.arrow_definition(start + found.end());
        }
        let mut index = start + found.end() - 1;
        if "[<".contains(self.text[index]) {
            index = self.skip_type_parameters(index)?;
        }
        let closed = self.skip_balanced(index + 1)?;
        self.body_after_parameters(language, closed)
    }

    fn arrow_definition(&self, index: usize) -> Option<Body> {
        let mut index = self.significant(index);
        if self.word_at(index) == "async" {
            index = self.significant(index + "async".len());
        }
        if self.word_at(index) == "function" {
            let opener = self.find("(", index)?;
            let closed = self.skip_balanced(opener + 1)?;
            return self.body_after_parameters("TypeScript", closed);
        }
        if self.at(index) == Some('(') {
            let closed = self.skip_balanced(index + 1)?;
            return self.arrow_after_parameters(closed);
        }
        let parameter = self.word_at(index);
        if parameter.is_empty() {
            return None;
        }
        let index = self.significant(index + parameter.chars().count());
        self.starts_with("=>", index)
            .then(|| self.arrow_body(index + 2))
    }

    fn arrow_after_parameters(&self, mut index: usize) -> Option<Body> {
        let mut annotated = false;
        loop {
            index = self.significant(index);
            let c = self.at(index)?;
            if self.starts_with("=>", index) {
                return Some(self.arrow_body(index + 2));
            }
            if !annotated {
                if c != ':' {
                    return None;
                }
                annotated = true;
            } else if c == ';' || c == '=' {
                return None;
            } else if OPENERS.contains(c) {
                index = self.skip_balanced(index + 1)?;
                continue;
            }
            index += 1;
        }
    }

    fn arrow_body(&self, index: usize) -> Body {
        let index = self.significant(index);
        let kind = if self.at(index) == Some('{') {
            Kind::Block
        } else {
            Kind::Expression
        };
        (kind, index)
    }

    fn body_after_parameters(&self, language: &str, mut index: usize) -> Option<Body> {
        let script = language == "TypeScript" || language == "JavaScript";
        let mut annotated = false;
        loop {
            index = self.significant(index);
            let c = self.at(index)?;
            if c == '{' {
                return Some((Kind::Block, index));
            }
            if script && !annotated {
                if c != ':' {
                    return None;
                }
                annotated = true;
            } else if language == "Kotlin" && c == '=' && !self.starts_with("==", index) {
                return Some((Kind::Line, index + 1));
            } else if c == ';' || (script && c == '=') {
                return None;
            } else if OPENERS.contains(c) {
                index = self.skip_balanced(index + 1)?;
                continue;
            }
            index += 1;
        }
    }

    fn end(&self, (kind, mut index): Body) -> Option<usize> {
        if kind == Kind::Block {
            let closed = self.skip_balanced(index + 1)?;
            return Some(self.line_of(closed - 1));
        }
        while index < self.text.len() {
            if kind == Kind::Expression {
                index = self.significant(index);
                if index >= self.text.len() {
                    break;
                }
            }
            let c = self.text[index];
            if kind == Kind::Expression && c == ';' {
                return Some(self.line_of(index));
            }
            if kind == Kind::Line && c == '\n' {
                return Some(self.line_of(index));
            }
            if kind == Kind::Line && QUOTES.contains(c) {
                index = self.skip_string(index);
                continue;
            }
            if OPENERS.contains(c) {
                index = self.skip_balanced(index + 1)?;
                continue;
            }
            index += 1;
        }
        if kind == Kind::Expression {
            None
        } else {
            Some(self.line_of(self.text.len().saturating_sub(1)))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn source_of_size(label: &str, size: u64) -> PathBuf {
        let root = std::env::temp_dir().join(format!("dm-cite-{label}-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(&root).unwrap();
        let mut content = b"one\n".to_vec();
        content.resize(size as usize, b'x');
        std::fs::write(root.join("rule.md"), content).unwrap();
        py::resolve_path(&root)
    }

    #[test]
    fn a_citation_of_a_source_of_exactly_the_evidence_size_limit_is_built() {
        let root = source_of_size("at", MAX_EVIDENCE_BYTES);
        let reference = citation(&root, "rule.md", "1", "1").unwrap();
        let lines = reference.get("lines");
        let _ = std::fs::remove_dir_all(&root);
        let Some(Value::Object(lines)) = lines else {
            panic!("no lines in the citation");
        };
        assert_eq!(
            (lines.get("start"), lines.get("end")),
            (
                Some(&Value::Int("1".to_string())),
                Some(&Value::Int("1".to_string()))
            )
        );
    }

    #[test]
    fn a_citation_of_a_source_one_byte_over_the_evidence_size_limit_is_refused() {
        let root = source_of_size("over", MAX_EVIDENCE_BYTES + 1);
        let outcome = citation(&root, "rule.md", "1", "1");
        let _ = std::fs::remove_dir_all(&root);
        assert_eq!(
            outcome.unwrap_err(),
            "evidence source exceeds 16777216 bytes: rule.md"
        );
    }
}

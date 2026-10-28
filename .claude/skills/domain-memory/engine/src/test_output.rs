use crate::digits::decimal_value;
use crate::py::{self, is_space};
use crate::pyvalue::splitlines;
use crate::unicode::is_alphanumeric;

const OUTPUT_TAIL: usize = 1200;
const MAX_FAILURE_LINES: usize = 40;
const MAX_DETAIL_LINES: usize = 3;
const BUILD_FAILURES: [&str; 9] = [
    "declared and not used",
    "imported and not used",
    "[build failed]",
    "syntax error",
    "SyntaxError",
    "IndentationError",
    "could not compile",
    "COMPILATION ERROR",
    "error: cannot find symbol",
];
const FAILURE_LINE_STARTS: [&str; 5] = ["--- FAIL", "FAIL:", "ERROR:", "FAILED", "panic:"];
const PYTEST_KEYWORDS: [&str; 8] = [
    "passed",
    "failed",
    "skipped",
    "deselected",
    "xfailed",
    "xpassed",
    "errors",
    "warnings",
];

fn is_digit(character: char) -> bool {
    decimal_value(character).is_some()
}

fn is_word(character: char) -> bool {
    is_alphanumeric(character) || character == '_'
}

fn run_of(text: &str, member: impl Fn(char) -> bool) -> &str {
    let end = text.find(|c: char| !member(c)).unwrap_or(text.len());
    &text[..end]
}

fn line_starts(text: &str) -> impl Iterator<Item = usize> + '_ {
    std::iter::once(0).chain(text.match_indices('\n').map(|(at, _)| at + 1))
}

fn next_line_start(text: &str, from: usize) -> Option<usize> {
    text[from..].find('\n').map(|at| from + at + 1)
}

fn anchored_after_blanks(text: &str, at: usize) -> bool {
    let before = &text[..at];
    let trimmed = before.trim_end_matches(is_space);
    trimmed.is_empty() || before[trimmed.len()..].contains('\n')
}

fn count_of(text: &str) -> u128 {
    text.chars()
        .filter_map(decimal_value)
        .fold(0u128, |total, digit| {
            total.saturating_mul(10).saturating_add(u128::from(digit))
        })
}

pub fn build_failed(output: &str) -> bool {
    BUILD_FAILURES.iter().any(|marker| output.contains(marker))
        || output.match_indices("error TS").any(|(at, found)| {
            output[at + found.len()..]
                .chars()
                .next()
                .is_some_and(is_digit)
        })
}

fn failure_line(line: &str) -> bool {
    let trimmed = line.trim_start_matches(is_space);
    FAILURE_LINE_STARTS
        .iter()
        .any(|start| trimmed.starts_with(start))
}

fn go_failed_tests(output: &str) -> Vec<String> {
    const MARK: &str = "--- FAIL: ";
    let mut names = Vec::new();
    let mut resume = 0;
    for (at, _) in output.match_indices(MARK) {
        if at < resume || !anchored_after_blanks(output, at) {
            continue;
        }
        let after = at + MARK.len();
        let name = run_of(&output[after..], |c| !is_space(c));
        if name.is_empty() {
            continue;
        }
        resume = after + name.len();
        names.push(name.to_string());
    }
    names
}

fn go_json_failed_tests(output: &str) -> Vec<String> {
    const ACTION: &str = "\"Action\":\"fail\"";
    const TEST: &str = "\"Test\":\"";
    let mut names = Vec::new();
    let mut from = 0;
    while let Some(found) = output[from..].find(ACTION) {
        let action = from + found;
        let after = action + ACTION.len();
        let line_end = output[after..]
            .find('\n')
            .map_or(output.len(), |at| after + at);
        let mut matched = None;
        for (offset, _) in output[after..line_end].match_indices(TEST) {
            let start = after + offset + TEST.len();
            let Some(quote) = output[start..].find('"') else {
                break;
            };
            if quote > 0 {
                matched = Some((output[start..start + quote].to_string(), start + quote + 1));
                break;
            }
        }
        match matched {
            Some((name, end)) => {
                names.push(name);
                from = end;
            }
            None => from = action + 1,
        }
    }
    names
}

fn has_pair(token: &str) -> bool {
    (1..token.len()).any(|at| {
        token.is_char_boundary(at) && token[at..].starts_with("::") && at + 2 < token.len()
    })
}

fn pytest_failed_test(line: &str) -> Option<String> {
    for prefix in ["FAILED ", "ERROR "] {
        if let Some(rest) = line.strip_prefix(prefix) {
            let token = run_of(rest, |c| !is_space(c));
            if has_pair(token) {
                return Some(token.to_string());
            }
        }
    }
    let token = run_of(line, |c| !is_space(c));
    let rest = line[token.len()..].strip_prefix(' ')?;
    let verdict = rest.starts_with("FAILED") || rest.starts_with("ERROR");
    (has_pair(token) && verdict).then(|| token.to_string())
}

fn unittest_failed_test(line: &str) -> Option<(String, String)> {
    let rest = line
        .strip_prefix("FAIL: ")
        .or_else(|| line.strip_prefix("ERROR: "))?;
    let name = run_of(rest, is_word);
    let rest = rest[name.len()..].strip_prefix(" (")?;
    let location = run_of(rest, |c| is_word(c) || c == '.');
    let closed = rest[location.len()..].starts_with(')');
    (!name.is_empty() && !location.is_empty() && closed)
        .then(|| (name.to_string(), location.to_string()))
}

pub fn failing_tests(output: &str) -> Vec<String> {
    let mut names: Vec<String> = go_failed_tests(output)
        .into_iter()
        .chain(go_json_failed_tests(output))
        .map(|name| name.split('/').next().unwrap_or_default().to_string())
        .collect();
    let lines: Vec<&str> = output.split('\n').collect();
    names.extend(lines.iter().filter_map(|line| pytest_failed_test(line)));
    for (name, location) in lines.iter().filter_map(|line| unittest_failed_test(line)) {
        names.push(if location.ends_with(&format!(".{name}")) {
            location
        } else {
            format!("{location}.{name}")
        });
    }
    let mut unique: Vec<String> = Vec::new();
    for name in names {
        if !unique.contains(&name) {
            unique.push(name);
        }
    }
    unique
}

fn failure_lines(lines: &[&str]) -> Vec<String> {
    let mut kept = Vec::new();
    let mut depth: Option<usize> = None;
    let mut details = 0;
    for line in lines {
        let indent = line.chars().count() - line.trim_start_matches(is_space).chars().count();
        if failure_line(line) {
            kept.push((*line).to_string());
            depth = Some(indent);
            details = 0;
        } else if depth.is_some_and(|held| indent > held) && details < MAX_DETAIL_LINES {
            kept.push((*line).to_string());
            details += 1;
        } else {
            depth = None;
        }
    }
    kept
}

pub fn failure_evidence(output: &str) -> String {
    let total = output.chars().count();
    let (head, tail) = if total > OUTPUT_TAIL {
        let at = output
            .char_indices()
            .nth(total - OUTPUT_TAIL)
            .map_or(output.len(), |(at, _)| at);
        output.split_at(at)
    } else {
        ("", output)
    };
    let failures = failure_lines(&splitlines(head));
    if failures.is_empty() {
        return tail.to_string();
    }
    let mut shown: Vec<&str> = failures
        .iter()
        .take(MAX_FAILURE_LINES)
        .map(String::as_str)
        .collect();
    shown.push("...");
    shown.push(tail);
    shown.join("\n")
}

fn go_matches(output: &str, word: &str, action: &str) -> Vec<usize> {
    let mark = format!("--- {word}: ");
    let json = format!("\"Action\":\"{action}\"");
    let mut ends = Vec::new();
    let mut at = 0;
    while at < output.len() {
        let mut matched = None;
        let mut jump = None;
        if line_starts_here(output, at) {
            let blanks = run_of(&output[at..], is_space).len();
            if output[at + blanks..].starts_with(&mark) {
                matched = Some(at + blanks + mark.len());
            } else if blanks > 0 {
                jump = Some(at + blanks);
            }
        }
        if matched.is_none() && output[at..].starts_with(&json) {
            let after = at + json.len();
            let line_end = output[after..]
                .find('\n')
                .map_or(output.len(), |found| after + found);
            matched = output[after..line_end]
                .rfind("\"Test\":\"")
                .map(|found| after + found + "\"Test\":\"".len());
        }
        match (matched, jump) {
            (Some(end), _) => {
                ends.push(end);
                at = end;
            }
            (None, Some(target)) => at = target,
            (None, None) => at += output[at..].chars().next().map_or(1, char::len_utf8),
        }
    }
    ends
}

fn line_starts_here(text: &str, at: usize) -> bool {
    at == 0 || text.as_bytes()[at - 1] == b'\n'
}

fn go_test_passed(output: &str) -> bool {
    !go_matches(output, "PASS", "pass").is_empty()
}

fn go_skipped_count(output: &str) -> usize {
    go_matches(output, "SKIP", "skip").len()
}

fn go_skip_reason(output: &str) -> Option<String> {
    let mut at = 0;
    while at <= output.len() {
        if !line_starts_here(output, at) {
            at = next_line_start(output, at)?;
            continue;
        }
        let blanks = run_of(&output[at..], is_space).len();
        let start = at + blanks;
        if blanks > 0
            && let Some(reason) = skip_reason_at(output, start)
        {
            return Some(reason);
        }
        at = next_line_start(output, start.max(at))?;
    }
    None
}

fn skip_reason_at(output: &str, start: usize) -> Option<String> {
    let token = run_of(&output[start..], |c| !is_space(c));
    let after_token = start + token.len();
    let rest = output[after_token..].strip_prefix(' ')?;
    let core = token.strip_suffix(':')?;
    let marker = core.rfind(".go:")?;
    let digits = &core[marker + ".go:".len()..];
    if marker == 0 || digits.is_empty() || !digits.chars().all(is_digit) {
        return None;
    }
    let line_length = rest.find('\n')?;
    if line_length == 0 {
        return None;
    }
    let following = &rest[line_length + 1..];
    following
        .trim_start_matches(is_space)
        .starts_with("--- SKIP: ")
        .then(|| rest[..line_length].to_string())
}

fn unittest_ran(output: &str) -> Option<u128> {
    line_starts(output).find_map(|at| {
        let rest = output[at..].strip_prefix("Ran ")?;
        let digits = run_of(rest, is_digit);
        let rest = rest[digits.len()..].strip_prefix(" test")?;
        let rest = rest.strip_prefix('s').unwrap_or(rest);
        (!digits.is_empty() && rest.starts_with(" in ")).then(|| count_of(digits))
    })
}

fn unittest_skipped(output: &str) -> Option<u128> {
    output.match_indices("skipped=").find_map(|(at, found)| {
        let digits = run_of(&output[at + found.len()..], is_digit);
        (!digits.is_empty()).then(|| count_of(digits))
    })
}

fn pytest_group(text: &str) -> Option<(&str, usize)> {
    let count = run_of(text, is_digit);
    let mut rest = text[count.len()..].strip_prefix(' ')?;
    if count.is_empty() {
        return None;
    }
    let keyword = PYTEST_KEYWORDS.iter().find_map(|keyword| {
        let plural = rest.starts_with(keyword);
        let singular = matches!(*keyword, "errors" | "warnings")
            .then(|| &keyword[..keyword.len() - 1])
            .filter(|short| rest.starts_with(short));
        if plural {
            Some(keyword.len())
        } else {
            singular.map(str::len)
        }
    })?;
    rest = &rest[keyword..];
    while let Some(after) = rest.strip_prefix(", ") {
        let digits = run_of(after, is_digit);
        let Some(after) = after[digits.len()..].strip_prefix(' ') else {
            break;
        };
        let word = run_of(after, is_word);
        if digits.is_empty() || word.is_empty() {
            break;
        }
        rest = &after[word.len()..];
    }
    let group_end = text.len() - rest.len();
    let after = rest.strip_prefix(" in ")?;
    let seconds = run_of(after, |c| is_digit(c) || c == '.');
    if seconds.is_empty() || !after[seconds.len()..].starts_with('s') {
        return None;
    }
    let end = text.len() - after.len() + seconds.len() + 1;
    Some((&text[..group_end], end))
}

fn pytest_summaries(output: &str) -> Vec<String> {
    let mut found = Vec::new();
    let mut at = 0;
    loop {
        let rest = &output[at..];
        let equals = run_of(rest, |c| c == '=').len();
        let blanks = run_of(&rest[equals..], is_space).len();
        let group_at = at + equals + blanks;
        if let Some((group, end)) = pytest_group(&output[group_at..]) {
            found.push(group.to_string());
            let Some(next) = next_line_start(output, group_at + end) else {
                break;
            };
            at = next;
            continue;
        }
        let run_end = at + equals + blanks;
        let next = match output[at..run_end].rfind('\n') {
            Some(last) => Some(at + last + 1),
            None => next_line_start(output, run_end),
        };
        let Some(next) = next else {
            break;
        };
        at = next;
    }
    found
}

pub fn is_go_test(argv: &[String]) -> bool {
    let Some(first) = argv.first() else {
        return false;
    };
    stem_of(first) == "go" && argv[1..].iter().any(|argument| argument == "test")
}

fn stem_of(argument: &str) -> String {
    let separators: &[char] = if cfg!(windows) { &['/', '\\'] } else { &['/'] };
    let name = argument
        .trim_end_matches(separators)
        .rsplit(separators)
        .next()
        .unwrap_or_default();
    match name.rfind('.') {
        Some(dot) if dot > 0 && dot + 1 < name.len() => name[..dot].to_string(),
        _ => name.to_string(),
    }
}

pub fn reports_tests(argv: &[String]) -> bool {
    argv.iter().any(|argument| {
        argument == "-v"
            || argument == "-json"
            || argument.starts_with("-v=")
            || argument.starts_with("-test.v")
    })
}

fn no_go_test_passed(output: &str) -> String {
    let skipped = go_skipped_count(output);
    if skipped == 0 {
        return "no test passed: no test matched".to_string();
    }
    let said = go_skip_reason(output)
        .map(|reason| format!(", the first because {}", py::strip(&reason)))
        .unwrap_or_default();
    format!("no test passed: every test was skipped ({skipped}){said}")
}

pub fn no_test_passed(argv: &[String], output: &str) -> Option<String> {
    if is_go_test(argv) {
        if !reports_tests(argv) {
            return Some("go test without -v or -json does not say which tests passed, so a skipped test reads as a pass; add -v".to_string());
        }
        return if go_test_passed(output) {
            None
        } else {
            Some(no_go_test_passed(output))
        };
    }
    let nothing_ran = output.contains("[no tests to run]") || output.contains("[no test files]");
    if nothing_ran && !go_test_passed(output) {
        return Some("go test found no test to run".to_string());
    }
    if let Some(ran) = unittest_ran(output) {
        let skipped = unittest_skipped(output);
        if ran == 0 || skipped.is_some_and(|count| count >= ran) {
            return Some("unittest ran no test that was not skipped".to_string());
        }
    }
    let summaries = pytest_summaries(output);
    match summaries.last() {
        Some(last) if !last.contains("passed") => Some(format!("pytest passed no test: {last}")),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_pytest_summary_names_its_counts_without_the_rule_lines() {
        assert_eq!(
            pytest_summaries("== 3 passed, 1 skipped in 0.12s ==\n1 error in 4s\n"),
            ["3 passed, 1 skipped", "1 error"]
        );
        assert!(pytest_summaries("3 passed in x").is_empty());
    }

    #[test]
    fn the_stem_of_a_command_drops_one_extension() {
        assert_eq!(stem_of("/usr/bin/go.exe"), "go");
        assert_eq!(stem_of("go"), "go");
        assert_eq!(stem_of(".go"), ".go");
    }

    fn golden() -> crate::json::Object {
        let text = include_str!("test_output_golden.json");
        let crate::json::Value::Object(object) = crate::json::parse(text).unwrap() else {
            panic!("golden file is not an object");
        };
        object
    }

    fn strings(value: &crate::json::Value) -> Vec<String> {
        value
            .as_array()
            .unwrap()
            .iter()
            .map(|item| item.as_str().unwrap().to_string())
            .collect()
    }

    fn list<'a>(golden: &'a crate::json::Object, key: &str) -> &'a [crate::json::Value] {
        golden.get(key).unwrap().as_array().unwrap()
    }

    #[test]
    fn failing_tests_and_evidence_match_the_recorded_python_results() {
        let golden = golden();
        let outputs = strings(golden.get("outputs").unwrap());
        for (index, output) in outputs.iter().enumerate() {
            assert_eq!(
                failing_tests(output),
                strings(&list(&golden, "failing_tests")[index]),
                "failing_tests of output {index}"
            );
            assert_eq!(
                failure_evidence(output),
                list(&golden, "failure_evidence")[index].as_str().unwrap(),
                "failure_evidence of output {index}"
            );
        }
    }

    #[test]
    fn the_go_test_verdicts_match_the_recorded_python_results() {
        let golden = golden();
        let outputs = strings(golden.get("outputs").unwrap());
        let argvs: Vec<Vec<String>> = list(&golden, "argvs").iter().map(strings).collect();
        for (index, argv) in argvs.iter().enumerate() {
            let crate::json::Value::Bool(go) = list(&golden, "is_go_test")[index] else {
                panic!("is_go_test is not a bool");
            };
            let crate::json::Value::Bool(reports) = list(&golden, "reports_tests")[index] else {
                panic!("reports_tests is not a bool");
            };
            assert_eq!(is_go_test(argv), go, "is_go_test of {argv:?}");
            assert_eq!(reports_tests(argv), reports, "reports_tests of {argv:?}");
        }
        for row in list(&golden, "no_test_passed") {
            let row = row.as_array().unwrap();
            let (crate::json::Value::Int(a), crate::json::Value::Int(o)) = (&row[0], &row[1])
            else {
                panic!("indices are not integers");
            };
            let (argv, output) = (
                &argvs[a.parse::<usize>().unwrap()],
                &outputs[o.parse::<usize>().unwrap()],
            );
            let expected = row[2].as_str().map(str::to_string);
            assert_eq!(
                no_test_passed(argv, output),
                expected,
                "{argv:?} on {output:?}"
            );
        }
    }
}

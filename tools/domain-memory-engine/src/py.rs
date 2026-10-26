use std::cmp::Ordering;
use std::io;
use std::path::{Component, Path, PathBuf};

pub fn is_space(character: char) -> bool {
    character.is_whitespace() || ('\u{1c}'..='\u{1f}').contains(&character)
}

pub fn strip(text: &str) -> &str {
    text.trim_matches(is_space)
}

pub fn repr(text: &str) -> String {
    let quote = if text.contains('\'') && !text.contains('"') {
        '"'
    } else {
        '\''
    };
    let mut out = String::with_capacity(text.len() + 2);
    out.push(quote);
    for character in text.chars() {
        match character {
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            quoted if quoted == quote => {
                out.push('\\');
                out.push(quoted);
            }
            hidden if !is_printable(hidden) => out.push_str(&hidden_escape(hidden)),
            shown => out.push(shown),
        }
    }
    out.push(quote);
    out
}

fn is_printable(character: char) -> bool {
    let code = character as u32;
    !(code < 0x20
        || (0x7f..=0xa0).contains(&code)
        || code == 0xad
        || (0x2000..=0x200f).contains(&code)
        || (0x2028..=0x202f).contains(&code)
        || (0x205f..=0x206f).contains(&code)
        || code == 0x3000
        || code == 0xfeff
        || (0xe000..=0xf8ff).contains(&code))
}

fn hidden_escape(character: char) -> String {
    let code = character as u32;
    match code {
        0..=0xff => format!("\\x{code:02x}"),
        0x100..=0xffff => format!("\\u{code:04x}"),
        _ => format!("\\U{code:08x}"),
    }
}

pub fn resolve(given: &str) -> PathBuf {
    let absolute = std::path::absolute(given).unwrap_or_else(|_| PathBuf::from(given));
    let lexical = normalized(&absolute);
    let mut existing = lexical.clone();
    let mut missing = Vec::new();
    loop {
        if let Ok(found) = std::fs::canonicalize(&existing) {
            let mut resolved = without_verbatim_prefix(found);
            for part in missing.iter().rev() {
                resolved.push(part);
            }
            return resolved;
        }
        match (
            existing.file_name().map(ToOwned::to_owned),
            existing.parent(),
        ) {
            (Some(name), Some(parent)) => {
                missing.push(name);
                existing = parent.to_path_buf();
            }
            _ => return lexical,
        }
    }
}

fn normalized(path: &Path) -> PathBuf {
    let mut out = PathBuf::new();
    for component in path.components() {
        match component {
            Component::CurDir => {}
            Component::ParentDir => {
                out.pop();
            }
            other => out.push(other.as_os_str()),
        }
    }
    out
}

fn without_verbatim_prefix(path: PathBuf) -> PathBuf {
    let text = path.to_string_lossy();
    if let Some(rest) = text.strip_prefix(r"\\?\UNC\") {
        return PathBuf::from(format!(r"\\{rest}"));
    }
    if let Some(rest) = text.strip_prefix(r"\\?\") {
        return PathBuf::from(rest);
    }
    path
}

pub fn shown(path: &Path) -> String {
    path.to_string_lossy().into_owned()
}

pub fn os_error(error: &io::Error, path: &Path) -> String {
    let text = error.to_string();
    let message = match error.raw_os_error() {
        Some(code) => {
            let bare = text
                .strip_suffix(&format!(" (os error {code})"))
                .unwrap_or(&text);
            let label = if cfg!(windows) { "WinError" } else { "Errno" };
            let trimmed = if cfg!(windows) {
                bare.trim_end_matches(|c: char| c <= ' ' || c == '.')
            } else {
                bare
            };
            format!("[{label} {code}] {trimmed}")
        }
        None => text,
    };
    format!("{message}: {}", repr(&shown(path)))
}

pub fn compare_paths(left: &Path, right: &Path) -> Ordering {
    if cfg!(windows) {
        shown(left).to_lowercase().cmp(&shown(right).to_lowercase())
    } else {
        left.as_os_str().cmp(right.as_os_str())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn repr_quotes_and_escapes_as_python_does() {
        assert_eq!(repr(r"C:\a b"), r"'C:\\a b'");
        assert_eq!(repr("it's"), "\"it's\"");
        assert_eq!(repr("both ' and \""), "'both \\' and \"'");
        assert_eq!(repr("tab\tnew\nbell\u{7}é"), "'tab\\tnew\\nbell\\x07é'");
    }

    #[test]
    fn strip_removes_what_python_calls_whitespace() {
        assert_eq!(strip("\u{1f} \u{a0}id\u{3000}\n"), "id");
    }
}

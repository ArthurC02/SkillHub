use std::cmp::Ordering;
use std::io;
use std::path::{Component, Path, PathBuf};

pub enum Raised {
    Value(String),
    Os(String),
    Subprocess(String),
}

impl Raised {
    pub fn message(self) -> String {
        match self {
            Raised::Value(message) | Raised::Os(message) | Raised::Subprocess(message) => message,
        }
    }

    pub fn os(error: &io::Error, path: &Path) -> Self {
        Raised::Os(os_error(error, path))
    }

    pub fn open(error: &io::Error, path: &Path) -> Self {
        Raised::Os(open_error(error, path))
    }
}

impl From<String> for Raised {
    fn from(message: String) -> Self {
        Raised::Value(message)
    }
}

impl From<Raised> for String {
    fn from(raised: Raised) -> Self {
        raised.message()
    }
}

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

pub fn resolve_path(path: &Path) -> PathBuf {
    resolve(&path.to_string_lossy())
}

pub fn utf8_text(bytes: &[u8]) -> Result<String, String> {
    Ok(utf8_strict(bytes)?
        .replace("\r\n", "\n")
        .replace('\r', "\n"))
}

pub fn utf8_strict(bytes: &[u8]) -> Result<String, String> {
    std::str::from_utf8(bytes)
        .map(str::to_string)
        .map_err(|error| utf8_refusal(bytes, &error))
}

pub fn relative_to(path: &Path, top: &Path) -> Result<String, String> {
    let inside = path
        .ancestors()
        .any(|ancestor| compare_paths(ancestor, top) == Ordering::Equal);
    if !inside {
        return Err(format!(
            "{} is not in the subpath of {}",
            repr(&shown(path)),
            repr(&shown(top))
        ));
    }
    let skipped = top.components().count();
    Ok(path
        .components()
        .skip(skipped)
        .map(|part| part.as_os_str().to_string_lossy().into_owned())
        .collect::<Vec<_>>()
        .join("/"))
}

fn utf8_refusal(bytes: &[u8], error: &std::str::Utf8Error) -> String {
    let start = error.valid_up_to();
    let (end, reason) = match error.error_len() {
        None => (bytes.len(), "unexpected end of data"),
        Some(1) if matches!(bytes[start], 0x80..=0xC1 | 0xF5..=0xFF) => {
            (start + 1, "invalid start byte")
        }
        Some(length) => (start + length, "invalid continuation byte"),
    };
    if end - start == 1 {
        format!(
            "'utf-8' codec can't decode byte 0x{:02x} in position {start}: {reason}",
            bytes[start]
        )
    } else {
        format!(
            "'utf-8' codec can't decode bytes in position {start}-{}: {reason}",
            end - 1
        )
    }
}

pub fn resolve(given: &str) -> PathBuf {
    let given = if given.is_empty() { "." } else { given };
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

pub fn open_error(error: &io::Error, path: &Path) -> String {
    let (errno, text) = match error.raw_os_error() {
        Some(2 | 3) if cfg!(windows) => (2, "No such file or directory"),
        Some(5 | 32 | 33) if cfg!(windows) => (13, "Permission denied"),
        Some(80 | 183) if cfg!(windows) => (17, "File exists"),
        Some(87 | 123 | 161) if cfg!(windows) => (22, "Invalid argument"),
        Some(4) if cfg!(windows) => (24, "Too many open files"),
        _ => return os_error(error, path),
    };
    format!("[Errno {errno}] {text}: {}", repr(&shown(path)))
}

pub struct PurePath {
    anchor: String,
    parts: Vec<String>,
}

impl PurePath {
    pub fn new(given: &str) -> Self {
        if cfg!(windows) {
            Self::windows(given)
        } else {
            Self::posix(given)
        }
    }

    fn posix(given: &str) -> Self {
        let slashes = given.len() - given.trim_start_matches('/').len();
        let anchor = match slashes {
            0 => "",
            2 => "//",
            _ => "/",
        };
        Self {
            anchor: anchor.to_string(),
            parts: given
                .split('/')
                .filter(|part| !part.is_empty() && *part != ".")
                .map(str::to_string)
                .collect(),
        }
    }

    fn windows(given: &str) -> Self {
        let text = given.replace('/', "\\");
        let (drive, rest) = windows_drive(&text);
        let root = rest.starts_with('\\');
        Self {
            anchor: format!("{drive}{}", if root { "\\" } else { "" }),
            parts: rest
                .split('\\')
                .filter(|part| !part.is_empty() && *part != ".")
                .map(str::to_string)
                .collect(),
        }
    }

    pub fn name(&self) -> String {
        self.parts.last().cloned().unwrap_or_default()
    }

    pub fn parent(&self) -> Self {
        let mut parts = self.parts.clone();
        parts.pop();
        Self {
            anchor: self.anchor.clone(),
            parts,
        }
    }

    pub fn joined(&self, relative: &str) -> Self {
        let separator = if cfg!(windows) { "\\" } else { "/" };
        let own = format!("{}{}", self.anchor, self.parts.join(separator));
        Self::new(&Path::new(&own).join(relative).to_string_lossy())
    }

    pub fn native(&self) -> String {
        let separator = if cfg!(windows) { "\\" } else { "/" };
        let joined = format!("{}{}", self.anchor, self.parts.join(separator));
        if joined.is_empty() {
            ".".to_string()
        } else {
            joined
        }
    }

    pub fn as_posix(&self) -> String {
        let separator = if cfg!(windows) { "\\" } else { "/" };
        let joined = format!("{}{}", self.anchor, self.parts.join(separator));
        if joined.is_empty() {
            ".".to_string()
        } else {
            joined.replace('\\', "/")
        }
    }
}

fn windows_drive(text: &str) -> (String, &str) {
    let mut letters = text.chars();
    if let (Some(letter), Some(':')) = (letters.next(), letters.next())
        && letter.is_ascii_alphabetic()
    {
        return (text[..2].to_string(), &text[2..]);
    }
    if let Some(unc) = text.strip_prefix("\\\\") {
        let mut names = unc.splitn(3, '\\');
        if let (Some(server), Some(share)) = (names.next(), names.next())
            && !server.is_empty()
            && !share.is_empty()
        {
            let length = 2 + server.len() + 1 + share.len();
            return (text[..length].to_string(), &text[length..]);
        }
    }
    (String::new(), text)
}

pub fn compare_paths(left: &Path, right: &Path) -> Ordering {
    let parts = |path: &Path| -> Vec<String> {
        path.components()
            .map(|part| {
                let text = part.as_os_str().to_string_lossy().into_owned();
                if cfg!(windows) {
                    text.to_lowercase()
                } else {
                    text
                }
            })
            .collect()
    };
    parts(left).cmp(&parts(right))
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
    fn a_pure_path_drops_dots_and_repeated_separators_and_names_its_parent() {
        let given = if cfg!(windows) {
            "C:\\a/./b//"
        } else {
            "/a/./b//"
        };
        let prefix = if cfg!(windows) { "C:/" } else { "/" };
        assert_eq!(PurePath::new(given).as_posix(), format!("{prefix}a/b"));
        assert_eq!(
            PurePath::new(given).parent().as_posix(),
            format!("{prefix}a")
        );
        assert_eq!(PurePath::new("x").parent().as_posix(), ".");
        assert_eq!(PurePath::new("").as_posix(), ".");
    }

    #[test]
    fn paths_order_by_their_parts_not_by_their_text() {
        let before = Path::new("root/zz/x.md");
        let after = Path::new("root/zz-q/x.md");
        assert_eq!(compare_paths(before, after), Ordering::Less);
        assert_eq!(compare_paths(after, before), Ordering::Greater);
    }

    #[test]
    fn strip_removes_what_python_calls_whitespace() {
        assert_eq!(strip("\u{1f} \u{a0}id\u{3000}\n"), "id");
    }
}

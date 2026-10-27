use crate::py::is_space;
use crate::unicode::is_alphanumeric;

pub type Matcher = fn(&[char]) -> bool;

pub const PATTERNS: [(&str, Matcher); 6] = [
    ("private-key", private_key),
    ("github-token", github_token),
    ("gitlab-token", gitlab_token),
    ("aws-access-key", aws_access_key),
    ("slack-token", slack_token),
    ("generic-secret", generic_secret),
];

const KEY_OPENING: &str = "-----BEGIN ";
const KEY_KINDS: [&str; 4] = ["", "RSA ", "EC ", "OPENSSH "];
const KEY_CLOSING: &str = "PRIVATE KEY-----";
const LONG_TOKEN_RUN: usize = 20;
const SLACK_TOKEN_RUN: usize = 10;
const MINIMUM_SECRET_RUN: usize = 16;
const AWS_KEY_BODY: usize = 16;

fn is_word(character: char) -> bool {
    character == '_' || is_alphanumeric(character)
}

fn boundary(text: &[char], at: usize) -> bool {
    let before = at > 0 && is_word(text[at - 1]);
    let after = text.get(at).copied().is_some_and(is_word);
    before != after
}

fn literal_at(text: &[char], at: usize, literal: &str) -> bool {
    literal
        .chars()
        .enumerate()
        .all(|(offset, expected)| text.get(at + offset) == Some(&expected))
}

fn letters_at(text: &[char], at: usize, letters: &str) -> bool {
    letters.chars().enumerate().all(|(offset, letter)| {
        text.get(at + offset)
            .is_some_and(|found| same_letter(*found, letter))
    })
}

fn same_letter(found: char, letter: char) -> bool {
    found.to_ascii_lowercase() == letter
        || matches!(
            (letter, found),
            ('i', '\u{130}' | '\u{131}') | ('s', '\u{17f}') | ('k', '\u{212a}')
        )
}

fn run_length(text: &[char], from: usize, member: fn(char) -> bool) -> usize {
    text[from.min(text.len())..]
        .iter()
        .take_while(|character| member(**character))
        .count()
}

fn word_or_hyphen(character: char) -> bool {
    character.is_ascii_alphanumeric() || character == '_' || character == '-'
}

fn ascii_word(character: char) -> bool {
    character.is_ascii_alphanumeric() || character == '_'
}

fn secret_character(character: char) -> bool {
    word_or_hyphen(character) || matches!(character, '\u{130}' | '\u{131}' | '\u{17f}' | '\u{212a}')
}

fn is_quote(character: char) -> bool {
    character == '\'' || character == '"'
}

fn bounded_run(text: &[char], start: usize, member: fn(char) -> bool, minimum: usize) -> bool {
    let length = run_length(text, start, member);
    (minimum..=length).any(|taken| boundary(text, start + taken))
}

fn private_key(text: &[char]) -> bool {
    (0..text.len()).any(|at| {
        literal_at(text, at, KEY_OPENING)
            && KEY_KINDS.iter().any(|kind| {
                let after_kind = at + KEY_OPENING.len() + kind.len();
                literal_at(text, at + KEY_OPENING.len(), kind)
                    && literal_at(text, after_kind, KEY_CLOSING)
            })
    })
}

fn github_token(text: &[char]) -> bool {
    (0..text.len()).any(|at| {
        boundary(text, at)
            && literal_at(text, at, "gh")
            && text
                .get(at + 2)
                .is_some_and(|kind| matches!(kind, 'p' | 'o' | 'u' | 's' | 'r'))
            && text.get(at + 3) == Some(&'_')
            && bounded_run(text, at + 4, ascii_word, LONG_TOKEN_RUN)
    })
}

fn gitlab_token(text: &[char]) -> bool {
    (0..text.len()).any(|at| {
        boundary(text, at)
            && literal_at(text, at, "glpat-")
            && bounded_run(text, at + 6, word_or_hyphen, LONG_TOKEN_RUN)
    })
}

fn aws_access_key(text: &[char]) -> bool {
    (0..text.len()).any(|at| {
        boundary(text, at)
            && (literal_at(text, at, "AKIA") || literal_at(text, at, "ASIA"))
            && run_length(text, at + 4, |character| {
                character.is_ascii_digit() || character.is_ascii_uppercase()
            }) >= AWS_KEY_BODY
            && boundary(text, at + 4 + AWS_KEY_BODY)
    })
}

fn slack_token(text: &[char]) -> bool {
    (0..text.len()).any(|at| {
        boundary(text, at)
            && literal_at(text, at, "xox")
            && text
                .get(at + 3)
                .is_some_and(|kind| matches!(kind, 'a' | 'b' | 'p' | 'r' | 's'))
            && text.get(at + 4) == Some(&'-')
            && bounded_run(text, at + 5, alphanumeric_or_hyphen, SLACK_TOKEN_RUN)
    })
}

fn alphanumeric_or_hyphen(character: char) -> bool {
    character.is_ascii_alphanumeric() || character == '-'
}

fn generic_secret(text: &[char]) -> bool {
    (0..text.len()).any(|at| {
        boundary(text, at)
            && keyword_ends(text, at)
                .into_iter()
                .any(|end| assigned_value(text, end))
    })
}

fn keyword_ends(text: &[char], at: usize) -> Vec<usize> {
    let mut ends = Vec::new();
    for word in ["secret", "password", "token"] {
        if letters_at(text, at, word) {
            ends.push(at + word.len());
        }
    }
    if letters_at(text, at, "api") {
        let joined = at + 3;
        if matches!(text.get(joined), Some('_' | '-')) && letters_at(text, joined + 1, "key") {
            ends.push(joined + 4);
        }
        if letters_at(text, joined, "key") {
            ends.push(joined + 3);
        }
    }
    ends
}

fn after_spaces(text: &[char], from: usize) -> usize {
    from + run_length(text, from, is_space)
}

fn assigned_value(text: &[char], keyword_end: usize) -> bool {
    let sign = after_spaces(text, keyword_end);
    if !matches!(text.get(sign), Some(':' | '=')) {
        return false;
    }
    let value = after_spaces(text, sign + 1);
    match text.get(value) {
        Some(first) if is_quote(*first) => {
            let length = run_length(text, value + 1, secret_character);
            length >= MINIMUM_SECRET_RUN
                && text
                    .get(value + 1 + length)
                    .is_some_and(|closing| is_quote(*closing))
        }
        Some(_) => {
            let length = run_length(text, value, secret_character);
            length >= MINIMUM_SECRET_RUN
                && !matches!(text.get(value + length), Some('.' | '(' | '['))
        }
        None => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn chars(text: &str) -> Vec<char> {
        text.chars().collect()
    }

    #[test]
    fn a_github_token_needs_twenty_word_characters_and_a_boundary_on_both_sides() {
        let token = format!("ghp_{}", "a".repeat(20));
        assert!(github_token(&chars(&token)));
        assert!(!github_token(&chars(&format!("ghp_{}", "a".repeat(19)))));
        assert!(!github_token(&chars(&format!("x{token}"))));
        assert!(!github_token(&chars(&format!("{token}\u{e9}"))));
    }

    #[test]
    fn a_generic_secret_ends_at_a_call_or_index_but_not_at_a_quote() {
        let value = "a".repeat(16);
        assert!(generic_secret(&chars(&format!("password = {value}\n"))));
        assert!(!generic_secret(&chars(&format!("password = {value}(\n"))));
        assert!(generic_secret(&chars(&format!("TOKEN: '{value}'"))));
        assert!(!generic_secret(&chars(&format!("TOKEN: '{value}"))));
    }
}

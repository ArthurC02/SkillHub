const WHITESPACE: [char; 4] = [' ', '\t', '\r', '\n'];
const QUOTES: [char; 2] = ['\'', '"'];
const ESCAPE: char = '\\';

#[derive(Clone, Copy, PartialEq)]
enum State {
    Between,
    Word,
    Quoted(char),
    Escaped(EscapedFrom),
}

#[derive(Clone, Copy, PartialEq)]
enum EscapedFrom {
    Word,
    Quote(char),
}

pub fn split(command: &str) -> Result<Vec<String>, String> {
    let mut tokens = Vec::new();
    let mut token = String::new();
    let mut quoted = false;
    let mut state = State::Between;
    for character in command.chars().map(Some).chain([None]) {
        state = match (state, character) {
            (State::Between, None) => break,
            (State::Between, Some(c)) if WHITESPACE.contains(&c) => {
                emit(&mut tokens, &mut token, &mut quoted);
                State::Between
            }
            (State::Between | State::Word, Some(ESCAPE)) => State::Escaped(EscapedFrom::Word),
            (State::Between | State::Word, Some(c)) if QUOTES.contains(&c) => State::Quoted(c),
            (State::Between, Some(c)) => {
                token.push(c);
                State::Word
            }
            (State::Word, Some(c)) if !WHITESPACE.contains(&c) => {
                token.push(c);
                State::Word
            }
            (State::Word, None) => {
                emit(&mut tokens, &mut token, &mut quoted);
                break;
            }
            (State::Word, Some(_)) => {
                emit(&mut tokens, &mut token, &mut quoted);
                State::Between
            }
            (State::Quoted(_), None) => return Err("No closing quotation".to_string()),
            (State::Quoted(open), Some(c)) => {
                quoted = true;
                if c == open {
                    State::Word
                } else if c == ESCAPE && open == '"' {
                    State::Escaped(EscapedFrom::Quote(open))
                } else {
                    token.push(c);
                    state
                }
            }
            (State::Escaped(_), None) => return Err("No escaped character".to_string()),
            (State::Escaped(from), Some(c)) => match from {
                EscapedFrom::Word => {
                    token.push(c);
                    State::Word
                }
                EscapedFrom::Quote(open) => {
                    if c != ESCAPE && c != open {
                        token.push(ESCAPE);
                    }
                    token.push(c);
                    State::Quoted(open)
                }
            },
        };
    }
    if !token.is_empty() || quoted {
        tokens.push(token);
    }
    Ok(tokens)
}

fn emit(tokens: &mut Vec<String>, token: &mut String, quoted: &mut bool) {
    if !token.is_empty() || *quoted {
        tokens.push(std::mem::take(token));
    }
    *quoted = false;
}

#[cfg(test)]
mod tests {
    use super::split;

    #[test]
    fn words_quotes_and_escapes_split_as_a_posix_shell_would() {
        assert_eq!(
            split(r#"go test -run "A B" 'c d' e\ f """#).unwrap(),
            ["go", "test", "-run", "A B", "c d", "e f", ""]
        );
        assert_eq!(split(r#""a\"b\c""#).unwrap(), [r#"a"b\c"#]);
    }

    #[test]
    fn an_unfinished_quote_or_escape_is_refused() {
        assert_eq!(split("a 'b").unwrap_err(), "No closing quotation");
        assert_eq!(split("a\\").unwrap_err(), "No escaped character");
    }

    #[test]
    fn every_recorded_command_line_splits_as_python_split_it() {
        let crate::json::Value::Object(golden) =
            crate::json::parse(include_str!("test_output_golden.json")).unwrap()
        else {
            panic!("golden file is not an object");
        };
        let rows = golden.get("shlex").unwrap().as_array().unwrap();
        assert!(rows.len() > 40);
        for row in rows {
            let row = row.as_array().unwrap();
            let text = row[0].as_str().unwrap();
            let outcome = row[1].as_object().unwrap();
            let expected = match (outcome.get("value"), outcome.get("error")) {
                (Some(value), _) => Ok(value
                    .as_array()
                    .unwrap()
                    .iter()
                    .map(|word| word.as_str().unwrap().to_string())
                    .collect::<Vec<_>>()),
                (_, Some(error)) => Err(error.as_str().unwrap().to_string()),
                _ => panic!("outcome has neither value nor error"),
            };
            assert_eq!(split(text), expected, "splitting {text:?}");
        }
    }
}

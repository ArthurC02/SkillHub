enum Piece {
    Literal(char),
    Star,
    AnyChar,
    Never,
    Set(CharSet),
}

struct CharSet {
    negated: bool,
    singles: Vec<char>,
    ranges: Vec<(char, char)>,
}

impl CharSet {
    fn holds(&self, character: char) -> bool {
        let found = self.singles.contains(&character)
            || self
                .ranges
                .iter()
                .any(|(low, high)| (*low..=*high).contains(&character));
        found != self.negated
    }
}

pub fn fnmatchcase(name: &str, pattern: &str) -> Result<bool, String> {
    let pieces = translated(&pattern.chars().collect::<Vec<_>>())?;
    let name: Vec<char> = name.chars().collect();
    Ok(matches_from(&pieces, &name))
}

fn matches_from(pieces: &[Piece], name: &[char]) -> bool {
    let Some((piece, rest)) = pieces.split_first() else {
        return name.is_empty();
    };
    match piece {
        Piece::Star => (0..=name.len()).any(|skip| matches_from(rest, &name[skip..])),
        Piece::Never => false,
        other => match name.split_first() {
            Some((first, tail)) => single_matches(other, *first) && matches_from(rest, tail),
            None => false,
        },
    }
}

fn single_matches(piece: &Piece, character: char) -> bool {
    match piece {
        Piece::Literal(expected) => *expected == character,
        Piece::AnyChar => true,
        Piece::Set(set) => set.holds(character),
        Piece::Star | Piece::Never => false,
    }
}

fn translated(pattern: &[char]) -> Result<Vec<Piece>, String> {
    let mut pieces = Vec::new();
    let mut index = 0;
    while index < pattern.len() {
        let character = pattern[index];
        index += 1;
        match character {
            '*' => {
                if !matches!(pieces.last(), Some(Piece::Star)) {
                    pieces.push(Piece::Star);
                }
            }
            '?' => pieces.push(Piece::AnyChar),
            '[' => match bracket_end(pattern, index) {
                None => pieces.push(Piece::Literal('[')),
                Some(end) => {
                    pieces.push(set_piece(pattern, index, end)?);
                    index = end + 1;
                }
            },
            other => pieces.push(Piece::Literal(other)),
        }
    }
    Ok(pieces)
}

fn bracket_end(pattern: &[char], start: usize) -> Option<usize> {
    let mut end = start;
    if pattern.get(end) == Some(&'!') {
        end += 1;
    }
    if pattern.get(end) == Some(&']') {
        end += 1;
    }
    while end < pattern.len() && pattern[end] != ']' {
        end += 1;
    }
    (end < pattern.len()).then_some(end)
}

fn set_piece(pattern: &[char], start: usize, end: usize) -> Result<Piece, String> {
    let stuff = &pattern[start..end];
    let stuff: Vec<char> = if stuff.contains(&'-') {
        hyphenated(pattern, start, end)?
    } else {
        escaped_backslashes(stuff)
    };
    let stuff: Vec<char> = stuff
        .iter()
        .flat_map(|character| {
            if matches!(character, '&' | '~' | '|') {
                vec!['\\', *character]
            } else {
                vec![*character]
            }
        })
        .collect();
    if stuff.is_empty() {
        return Ok(Piece::Never);
    }
    if stuff == ['!'] {
        return Ok(Piece::AnyChar);
    }
    let mut body = stuff;
    if body[0] == '!' {
        body[0] = '^';
    } else if matches!(body[0], '^' | '[') {
        body.insert(0, '\\');
    }
    parsed_set(&body).map(Piece::Set)
}

fn escaped_backslashes(stuff: &[char]) -> Vec<char> {
    stuff
        .iter()
        .flat_map(|character| {
            if *character == '\\' {
                vec!['\\', '\\']
            } else {
                vec![*character]
            }
        })
        .collect()
}

fn hyphenated(pattern: &[char], start: usize, end: usize) -> Result<Vec<char>, String> {
    let mut chunks: Vec<Vec<char>> = Vec::new();
    let mut first = start;
    let mut search = if pattern[start] == '!' {
        start + 2
    } else {
        start + 1
    };
    while let Some(found) = (search..end).find(|at| pattern[*at] == '-') {
        chunks.push(pattern[first..found].to_vec());
        first = found + 1;
        search = found + 3;
    }
    let last = pattern[first..end].to_vec();
    if last.is_empty() {
        chunks
            .last_mut()
            .ok_or("list index out of range")?
            .push('-');
    } else {
        chunks.push(last);
    }
    for at in (1..chunks.len()).rev() {
        let (Some(tail), Some(head)) =
            (chunks[at - 1].last().copied(), chunks[at].first().copied())
        else {
            return Err("string index out of range".to_string());
        };
        if tail > head {
            let mut merged = chunks[at - 1].clone();
            merged.pop();
            merged.extend_from_slice(&chunks[at][1..]);
            chunks[at - 1] = merged;
            chunks.remove(at);
        }
    }
    let mut joined = Vec::new();
    for (at, chunk) in chunks.iter().enumerate() {
        if at > 0 {
            joined.push('-');
        }
        for character in chunk {
            match character {
                '\\' => joined.extend(['\\', '\\']),
                '-' => joined.extend(['\\', '-']),
                other => joined.push(*other),
            }
        }
    }
    Ok(joined)
}

fn parsed_set(body: &[char]) -> Result<CharSet, String> {
    let mut set = CharSet {
        negated: body.first() == Some(&'^'),
        singles: Vec::new(),
        ranges: Vec::new(),
    };
    let mut at = usize::from(set.negated);
    while at < body.len() {
        let low = literal_at(body, &mut at);
        if body.get(at) != Some(&'-') {
            set.singles.push(low);
            continue;
        }
        at += 1;
        if at >= body.len() {
            set.singles.extend([low, '-']);
            continue;
        }
        let high = literal_at(body, &mut at);
        if high < low {
            return Err(format!(
                "bad character range {low}-{high} at position {}",
                at.saturating_sub(1)
            ));
        }
        set.ranges.push((low, high));
    }
    Ok(set)
}

fn literal_at(body: &[char], at: &mut usize) -> char {
    let mut character = body[*at];
    *at += 1;
    if character == '\\' && *at < body.len() {
        character = body[*at];
        *at += 1;
    }
    character
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hit(name: &str, pattern: &str) -> bool {
        fnmatchcase(name, pattern).unwrap()
    }

    #[test]
    fn star_crosses_slashes_and_question_mark_takes_one_character() {
        assert!(hit("src/a/b.txt", "src/*.txt"));
        assert!(hit("ab", "a?"));
        assert!(!hit("a", "a?"));
    }

    #[test]
    fn sets_negate_range_and_keep_a_leading_bracket_literal() {
        assert!(hit("b", "[a-c]") && !hit("d", "[a-c]"));
        assert!(hit("d", "[!a-c]") && !hit("b", "[!a-c]"));
        assert!(hit("]", "[]a]") && hit("a", "[]a]"));
        assert!(hit("[", "[") && !hit("a", "[z-a]"));
    }
}

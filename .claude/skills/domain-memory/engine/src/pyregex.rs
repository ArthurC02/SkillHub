use crate::digits::decimal_value;
use crate::py;
use crate::unicode::is_alphanumeric;

const IGNORE_CASE: u8 = 1;
const MULTILINE: u8 = 2;
const DOT_ALL: u8 = 4;
const VERBOSE: u8 = 8;
const ASCII: u8 = 16;
const UNICODE: u8 = 32;
const LOCALE: u8 = 64;
const TYPE_FLAGS: u8 = ASCII | UNICODE | LOCALE;
const SPECIAL_CHARS: &str = ".\\[{()*+?^$|";
const REPEAT_CHARS: &str = "*+?{";
const WHITESPACE: &str = " \t\n\r\x0b\x0c";
const ESCAPED_CHARS: &str = "()[]{}?*+-|^$\\.&~# \t\n\r\x0b\x0c";

#[derive(Clone, Copy, PartialEq)]
enum Category {
    Digit,
    NotDigit,
    Space,
    NotSpace,
    Word,
    NotWord,
}

#[derive(Clone)]
enum SetItem {
    Char(char),
    Range(char, char),
    Category(Category),
}

#[derive(Clone, Copy, PartialEq)]
enum At {
    Beginning,
    End,
    BeginningString,
    EndString,
    Boundary,
    NonBoundary,
}

#[derive(Clone, Copy, PartialEq)]
enum Mode {
    Greedy,
    Lazy,
    Possessive,
}

#[derive(Clone)]
enum Node {
    Literal(char),
    Any,
    Set {
        negate: bool,
        items: Vec<SetItem>,
    },
    Category(Category),
    At(At),
    Group {
        index: Option<usize>,
        add: u8,
        del: u8,
        body: Vec<Node>,
    },
    Alt(Vec<Vec<Node>>),
    Repeat {
        min: usize,
        max: Option<usize>,
        mode: Mode,
        item: Vec<Node>,
    },
    BackRef(usize),
    Look {
        ahead: bool,
        negate: bool,
        body: Vec<Node>,
    },
    Atomic(Vec<Node>),
    Failure,
}

pub struct Regex {
    root: Vec<Node>,
    groups: usize,
    flags: u8,
    names: Vec<(String, usize)>,
}

pub struct Match {
    spans: Vec<Option<(usize, usize)>>,
    names: Vec<(String, usize)>,
}

impl Match {
    pub fn end(&self) -> usize {
        self.spans[0].map_or(0, |(_, end)| end)
    }

    pub fn named(&self, name: &str) -> Option<(usize, usize)> {
        let (_, index) = self.names.iter().find(|(known, _)| known == name)?;
        self.spans.get(*index).copied().flatten()
    }
}

pub fn escape(text: &str) -> String {
    text.chars()
        .flat_map(|character| {
            let escaped = ESCAPED_CHARS.contains(character).then_some('\\');
            escaped.into_iter().chain([character])
        })
        .collect()
}

struct Source {
    chars: Vec<char>,
    index: usize,
    next: Option<String>,
}

impl Source {
    fn new(pattern: &str) -> Result<Self, String> {
        let mut source = Self {
            chars: pattern.chars().collect(),
            index: 0,
            next: None,
        };
        source.advance()?;
        Ok(source)
    }

    fn error(&self, message: &str, position: isize) -> String {
        let at = usize::try_from(position).unwrap_or(0);
        let mut text = format!("{message} at position {at}");
        if self.chars.contains(&'\n') {
            let before = &self.chars[..at.min(self.chars.len())];
            let line = before.iter().filter(|c| **c == '\n').count() + 1;
            let column = match before.iter().rposition(|c| *c == '\n') {
                Some(found) => at - found,
                None => at + 1,
            };
            text.push_str(&format!(" (line {line}, column {column})"));
        }
        text
    }

    fn fail(&self, message: &str, offset: usize) -> String {
        self.error(message, self.tell() as isize - offset as isize)
    }

    fn advance(&mut self) -> Result<(), String> {
        let Some(first) = self.chars.get(self.index).copied() else {
            self.next = None;
            return Ok(());
        };
        let mut token = String::from(first);
        let mut index = self.index;
        if first == '\\' {
            index += 1;
            match self.chars.get(index) {
                Some(escaped) => token.push(*escaped),
                None => {
                    return Err(
                        self.error("bad escape (end of pattern)", self.chars.len() as isize - 1)
                    );
                }
            }
        }
        self.index = index + 1;
        self.next = Some(token);
        Ok(())
    }

    fn matches(&mut self, token: &str) -> Result<bool, String> {
        if self.next.as_deref() == Some(token) {
            self.advance()?;
            return Ok(true);
        }
        Ok(false)
    }

    fn get(&mut self) -> Result<Option<String>, String> {
        let this = self.next.clone();
        self.advance()?;
        Ok(this)
    }

    fn tell(&self) -> usize {
        self.index - self.next.as_ref().map_or(0, |token| token.chars().count())
    }

    fn seek(&mut self, index: usize) -> Result<(), String> {
        self.index = index;
        self.advance()
    }

    fn get_while(
        &mut self,
        count: usize,
        allowed: impl Fn(char) -> bool,
    ) -> Result<String, String> {
        let mut result = String::new();
        for _ in 0..count {
            let Some(token) = self.next.clone() else {
                break;
            };
            let mut chars = token.chars();
            match (chars.next(), chars.next()) {
                (Some(only), None) if allowed(only) => result.push(only),
                _ => break,
            }
            self.advance()?;
        }
        Ok(result)
    }

    fn get_until(&mut self, terminator: &str, name: &str) -> Result<String, String> {
        let mut result = String::new();
        loop {
            let token = self.next.clone();
            self.advance()?;
            match token {
                None if result.is_empty() => return Err(self.fail(&format!("missing {name}"), 0)),
                None => {
                    return Err(self.fail(
                        &format!("missing {terminator}, unterminated name"),
                        result.chars().count(),
                    ));
                }
                Some(found) if found == terminator => {
                    if result.is_empty() {
                        return Err(self.fail(&format!("missing {name}"), 1));
                    }
                    return Ok(result);
                }
                Some(found) => result.push_str(&found),
            }
        }
    }

    fn next_is(&self, allowed: impl Fn(char) -> bool) -> bool {
        self.next.as_ref().is_some_and(|token| {
            let mut chars = token.chars();
            matches!((chars.next(), chars.next()), (Some(only), None) if allowed(only))
        })
    }
}

struct State {
    flags: u8,
    group_names: Vec<(String, usize)>,
    group_count: usize,
    open_groups: Vec<usize>,
    lookbehind_groups: Option<usize>,
}

impl State {
    fn open_group(&mut self, name: Option<&str>) -> Result<usize, String> {
        let gid = self.group_count;
        self.group_count += 1;
        self.open_groups.push(gid);
        if let Some(name) = name {
            if let Some((_, earlier)) = self.group_names.iter().find(|(seen, _)| seen == name) {
                return Err(format!(
                    "redefinition of group name {} as group {gid}; was group {earlier}",
                    py::repr(name)
                ));
            }
            self.group_names.push((name.to_string(), gid));
        }
        Ok(gid)
    }

    fn close_group(&mut self, gid: usize) {
        self.open_groups.retain(|open| *open != gid);
    }

    fn check_group(&self, gid: usize) -> bool {
        !self.open_groups.contains(&gid)
    }

    fn check_lookbehind_group(&self, gid: usize, source: &Source) -> Result<(), String> {
        match self.lookbehind_groups {
            Some(limit) if gid >= limit => Err(source.fail(
                "cannot refer to group defined in the same lookbehind subpattern",
                0,
            )),
            _ => Ok(()),
        }
    }
}

const fn is_octal(character: char) -> bool {
    matches!(character, '0'..='7')
}

fn is_hex(character: char) -> bool {
    character.is_ascii_hexdigit()
}

fn is_digit(character: char) -> bool {
    character.is_ascii_digit()
}

fn category_of(escape: &str) -> Option<Category> {
    match escape {
        "\\d" => Some(Category::Digit),
        "\\D" => Some(Category::NotDigit),
        "\\s" => Some(Category::Space),
        "\\S" => Some(Category::NotSpace),
        "\\w" => Some(Category::Word),
        "\\W" => Some(Category::NotWord),
        _ => None,
    }
}

fn simple_escape(escape: &str) -> Option<char> {
    match escape {
        "\\a" => Some('\x07'),
        "\\b" => Some('\x08'),
        "\\f" => Some('\x0c'),
        "\\n" => Some('\n'),
        "\\r" => Some('\r'),
        "\\t" => Some('\t'),
        "\\v" => Some('\x0b'),
        "\\\\" => Some('\\'),
        _ => None,
    }
}

enum Escaped {
    Literal(char),
    Category(Category),
    At(At),
    BackRef(usize),
}

fn character_escape(
    source: &mut Source,
    escape: &mut String,
    letter: char,
) -> Result<Option<char>, String> {
    let (digits, count) = match letter {
        'x' => (2, 2),
        'u' => (4, 4),
        'U' => (8, 8),
        _ => return Ok(None),
    };
    escape.push_str(&source.get_while(count, is_hex)?);
    if escape.chars().count() != digits + 2 {
        let length = escape.chars().count();
        return Err(source.fail(&format!("incomplete escape {escape}"), length));
    }
    let code = u32::from_str_radix(&escape[2..], 16).unwrap_or(u32::MAX);
    match char::from_u32(code) {
        Some(found) => Ok(Some(found)),
        None => Err(source.fail(&format!("bad escape {escape}"), escape.chars().count())),
    }
}

fn octal_value(source: &Source, escape: &str) -> Result<char, String> {
    let code = u32::from_str_radix(&escape[1..], 8).unwrap_or(0);
    if code > 0o377 {
        return Err(source.fail(
            &format!("octal escape value {escape} outside of range 0-0o377"),
            escape.chars().count(),
        ));
    }
    Ok(char::from_u32(code).unwrap_or('\0'))
}

fn bad_escape(source: &Source, escape: &str) -> String {
    source.fail(&format!("bad escape {escape}"), escape.chars().count())
}

fn escape_in_expression(
    source: &mut Source,
    escape: &str,
    state: &State,
) -> Result<Escaped, String> {
    if let Some(category) = category_of(escape) {
        return Ok(Escaped::Category(category));
    }
    match escape {
        "\\A" => return Ok(Escaped::At(At::BeginningString)),
        "\\Z" => return Ok(Escaped::At(At::EndString)),
        "\\b" => return Ok(Escaped::At(At::Boundary)),
        "\\B" => return Ok(Escaped::At(At::NonBoundary)),
        _ => {}
    }
    if let Some(found) = simple_escape(escape) {
        return Ok(Escaped::Literal(found));
    }
    let mut escape = escape.to_string();
    let letter = escape.chars().nth(1).unwrap_or('\0');
    if let Some(found) = character_escape(source, &mut escape, letter)? {
        return Ok(Escaped::Literal(found));
    }
    if letter == '0' {
        escape.push_str(&source.get_while(2, is_octal)?);
        return Ok(Escaped::Literal(octal_value(source, &escape)?));
    }
    if is_digit(letter) {
        if source.next_is(is_digit) {
            escape.push_str(&source.get()?.unwrap_or_default());
            let second = escape.chars().nth(2).unwrap_or('\0');
            if is_octal(letter) && is_octal(second) && source.next_is(is_octal) {
                escape.push_str(&source.get()?.unwrap_or_default());
                return Ok(Escaped::Literal(octal_value(source, &escape)?));
            }
        }
        let group: usize = escape[1..].parse().unwrap_or(usize::MAX);
        if group < state.group_count {
            if !state.check_group(group) {
                return Err(source.fail("cannot refer to an open group", escape.chars().count()));
            }
            state.check_lookbehind_group(group, source)?;
            return Ok(Escaped::BackRef(group));
        }
        return Err(source.fail(
            &format!("invalid group reference {group}"),
            escape.chars().count() - 1,
        ));
    }
    if escape.chars().count() == 2 && !letter.is_ascii_alphabetic() {
        return Ok(Escaped::Literal(letter));
    }
    Err(bad_escape(source, &escape))
}

fn escape_in_class(source: &mut Source, escape: &str) -> Result<SetItem, String> {
    if let Some(found) = simple_escape(escape) {
        return Ok(SetItem::Char(found));
    }
    if let Some(category) = category_of(escape) {
        return Ok(SetItem::Category(category));
    }
    let mut escape = escape.to_string();
    let letter = escape.chars().nth(1).unwrap_or('\0');
    if let Some(found) = character_escape(source, &mut escape, letter)? {
        return Ok(SetItem::Char(found));
    }
    if is_octal(letter) {
        escape.push_str(&source.get_while(2, is_octal)?);
        return Ok(SetItem::Char(octal_value(source, &escape)?));
    }
    if is_digit(letter) {
        return Err(bad_escape(source, &escape));
    }
    if escape.chars().count() == 2 && !letter.is_ascii_alphabetic() {
        return Ok(SetItem::Char(letter));
    }
    Err(bad_escape(source, &escape))
}

fn flag_bit(character: char) -> Option<u8> {
    match character {
        'i' => Some(IGNORE_CASE),
        'L' => Some(LOCALE),
        'm' => Some(MULTILINE),
        's' => Some(DOT_ALL),
        'x' => Some(VERBOSE),
        'a' => Some(ASCII),
        'u' => Some(UNICODE),
        _ => None,
    }
}

fn single(token: &Option<String>) -> Option<char> {
    let token = token.as_ref()?;
    let mut chars = token.chars();
    match (chars.next(), chars.next()) {
        (Some(only), None) => Some(only),
        _ => None,
    }
}

fn parse_flags(
    source: &mut Source,
    state: &mut State,
    first: char,
) -> Result<Option<(u8, u8)>, String> {
    let mut add = 0u8;
    let mut del = 0u8;
    let mut token = Some(first.to_string());
    if first != '-' {
        loop {
            let character = single(&token).unwrap_or('\0');
            let flag = flag_bit(character).unwrap_or(0);
            if character == 'L' {
                return Err(source.fail(
                    "bad inline flags: cannot use 'L' flag with a str pattern",
                    0,
                ));
            }
            add |= flag;
            if flag & TYPE_FLAGS != 0 && add & TYPE_FLAGS != flag {
                return Err(source.fail(
                    "bad inline flags: flags 'a', 'u' and 'L' are incompatible",
                    0,
                ));
            }
            token = source.get()?;
            let Some(character) = single(&token) else {
                return Err(source.fail("missing -, : or )", 0));
            };
            if ")-:".contains(character) {
                break;
            }
            if flag_bit(character).is_none() {
                let message = if character.is_alphabetic() {
                    "unknown flag"
                } else {
                    "missing -, : or )"
                };
                return Err(source.fail(message, 1));
            }
        }
    }
    if single(&token) == Some(')') {
        state.flags |= add;
        return Ok(None);
    }
    if single(&token) == Some('-') {
        token = source.get()?;
        let Some(character) = single(&token) else {
            return Err(source.fail("missing flag", 0));
        };
        if flag_bit(character).is_none() {
            let message = if character.is_alphabetic() {
                "unknown flag"
            } else {
                "missing flag"
            };
            return Err(source.fail(message, 1));
        }
        loop {
            let character = single(&token).unwrap_or('\0');
            let flag = flag_bit(character).unwrap_or(0);
            if flag & TYPE_FLAGS != 0 {
                return Err(source.fail(
                    "bad inline flags: cannot turn off flags 'a', 'u' and 'L'",
                    0,
                ));
            }
            del |= flag;
            token = source.get()?;
            let Some(character) = single(&token) else {
                return Err(source.fail("missing :", 0));
            };
            if character == ':' {
                break;
            }
            if flag_bit(character).is_none() {
                let message = if character.is_alphabetic() {
                    "unknown flag"
                } else {
                    "missing :"
                };
                return Err(source.fail(message, 1));
            }
        }
    }
    if add & del != 0 {
        return Err(source.fail("bad inline flags: flag turned on and off", 1));
    }
    Ok(Some((add, del)))
}

fn is_identifier(name: &str) -> bool {
    let mut chars = name.chars();
    chars
        .next()
        .is_some_and(|first| first == '_' || first.is_alphabetic())
        && chars.all(|c| c == '_' || c.is_alphanumeric())
}

fn parse_set(source: &mut Source, here: usize) -> Result<Node, String> {
    let negate = source.matches("^")?;
    let mut items: Vec<SetItem> = Vec::new();
    loop {
        let this = source.get()?;
        let Some(this) = this else {
            return Err(source.fail("unterminated character set", source.tell() - here));
        };
        if this == "]" && !items.is_empty() {
            break;
        }
        let first = if this.starts_with('\\') {
            escape_in_class(source, &this)?
        } else {
            SetItem::Char(this.chars().next().unwrap_or('\0'))
        };
        if source.matches("-")? {
            let that = source.get()?;
            let Some(that) = that else {
                return Err(source.fail("unterminated character set", source.tell() - here));
            };
            if that == "]" {
                items.push(first);
                items.push(SetItem::Char('-'));
                break;
            }
            let second = if that.starts_with('\\') {
                escape_in_class(source, &that)?
            } else {
                SetItem::Char(that.chars().next().unwrap_or('\0'))
            };
            let range = match (&first, &second) {
                (SetItem::Char(low), SetItem::Char(high)) if high >= low => {
                    Some(SetItem::Range(*low, *high))
                }
                _ => None,
            };
            match range {
                Some(range) => items.push(range),
                None => {
                    let message = format!("bad character range {this}-{that}");
                    let width = this.chars().count() + 1 + that.chars().count();
                    return Err(source.fail(&message, width));
                }
            }
        } else {
            items.push(first);
        }
    }
    Ok(Node::Set { negate, items })
}

fn parse_sub(
    source: &mut Source,
    state: &mut State,
    verbose: bool,
    nested: usize,
) -> Result<Vec<Node>, String> {
    let mut alternatives: Vec<Vec<Node>> = Vec::new();
    let mut verbose = verbose;
    loop {
        let first = nested == 0 && alternatives.is_empty();
        alternatives.push(parse_sequence(source, state, verbose, nested + 1, first)?);
        if !source.matches("|")? {
            break;
        }
        if nested == 0 {
            verbose = state.flags & VERBOSE != 0;
        }
    }
    if alternatives.len() == 1 {
        return Ok(alternatives.remove(0));
    }
    Ok(vec![Node::Alt(alternatives)])
}

fn parse_quantifier(
    source: &mut Source,
    this: &str,
    sequence: &mut Vec<Node>,
) -> Result<bool, String> {
    let here = source.tell();
    let (mut min, mut max) = (0usize, None);
    match this {
        "?" => max = Some(1),
        "*" => {}
        "+" => min = 1,
        _ => {
            if source.next.as_deref() == Some("}") {
                sequence.push(Node::Literal('{'));
                return Ok(false);
            }
            let mut low = String::new();
            let mut high = String::new();
            while source.next_is(is_digit) {
                low.push_str(&source.get()?.unwrap_or_default());
            }
            if source.matches(",")? {
                while source.next_is(is_digit) {
                    high.push_str(&source.get()?.unwrap_or_default());
                }
            } else {
                high.clone_from(&low);
            }
            if !source.matches("}")? {
                sequence.push(Node::Literal('{'));
                source.seek(here)?;
                return Ok(false);
            }
            if !low.is_empty() {
                min = low.parse().unwrap_or(usize::MAX);
            }
            if !high.is_empty() {
                let upper: usize = high.parse().unwrap_or(usize::MAX);
                if upper < min {
                    return Err(
                        source.fail("min repeat greater than max repeat", source.tell() - here)
                    );
                }
                max = Some(upper);
            }
        }
    }
    let repeated = matches!(sequence.last(), Some(Node::Repeat { .. }));
    if sequence.is_empty() || matches!(sequence.last(), Some(Node::At(_))) {
        return Err(source.fail("nothing to repeat", source.tell() - here + 1));
    }
    if repeated {
        return Err(source.fail("multiple repeat", source.tell() - here + 1));
    }
    let item = match sequence.pop() {
        Some(Node::Group {
            index: None,
            add: 0,
            del: 0,
            body,
        }) => body,
        Some(other) => vec![other],
        None => Vec::new(),
    };
    let mode = if source.matches("?")? {
        Mode::Lazy
    } else if source.matches("+")? {
        Mode::Possessive
    } else {
        Mode::Greedy
    };
    sequence.push(Node::Repeat {
        min,
        max,
        mode,
        item,
    });
    Ok(true)
}

fn parse_group(
    source: &mut Source,
    state: &mut State,
    verbose: bool,
    nested: usize,
    sequence: &mut Vec<Node>,
    first: bool,
) -> Result<(), String> {
    let start = source.tell() - 1;
    let mut capture = true;
    let mut atomic = false;
    let mut name: Option<String> = None;
    let (mut add, mut del) = (0u8, 0u8);
    let mut verbose = verbose;
    if source.matches("?")? {
        let token = source.get()?;
        let Some(token) = token else {
            return Err(source.fail("unexpected end of pattern", 0));
        };
        let character = single(&Some(token.clone())).unwrap_or('\0');
        match character {
            'P' => {
                if source.matches("<")? {
                    let found = source.get_until(">", "group name")?;
                    if !is_identifier(&found) {
                        let message = format!("bad character in group name {}", py::repr(&found));
                        return Err(source.fail(&message, found.chars().count() + 1));
                    }
                    name = Some(found);
                } else if source.matches("=")? {
                    let found = source.get_until(")", "group name")?;
                    let width = found.chars().count() + 1;
                    if !is_identifier(&found) {
                        let message = format!("bad character in group name {}", py::repr(&found));
                        return Err(source.fail(&message, width));
                    }
                    let Some((_, gid)) = state.group_names.iter().find(|(seen, _)| *seen == found)
                    else {
                        let message = format!("unknown group name {}", py::repr(&found));
                        return Err(source.fail(&message, width));
                    };
                    let gid = *gid;
                    if !state.check_group(gid) {
                        return Err(source.fail("cannot refer to an open group", width));
                    }
                    state.check_lookbehind_group(gid, source)?;
                    sequence.push(Node::BackRef(gid));
                    return Ok(());
                } else {
                    let after = source.get()?;
                    let Some(after) = after else {
                        return Err(source.fail("unexpected end of pattern", 0));
                    };
                    let message = format!("unknown extension ?P{after}");
                    return Err(source.fail(&message, after.chars().count() + 2));
                }
            }
            ':' => capture = false,
            '#' => {
                loop {
                    if source.next.is_none() {
                        return Err(
                            source.fail("missing ), unterminated comment", source.tell() - start)
                        );
                    }
                    if source.get()?.as_deref() == Some(")") {
                        break;
                    }
                }
                return Ok(());
            }
            '=' | '!' | '<' => {
                let mut kind = character;
                let mut ahead = true;
                let mut outer_lookbehind = None;
                if character == '<' {
                    let after = source.get()?;
                    let Some(after) = after else {
                        return Err(source.fail("unexpected end of pattern", 0));
                    };
                    kind = single(&Some(after.clone())).unwrap_or('\0');
                    if kind != '=' && kind != '!' {
                        let message = format!("unknown extension ?<{after}");
                        return Err(source.fail(&message, after.chars().count() + 2));
                    }
                    ahead = false;
                    outer_lookbehind = state.lookbehind_groups;
                    if outer_lookbehind.is_none() {
                        state.lookbehind_groups = Some(state.group_count);
                    }
                }
                let body = parse_sub(source, state, verbose, nested + 1)?;
                if !ahead && outer_lookbehind.is_none() {
                    state.lookbehind_groups = None;
                }
                if !source.matches(")")? {
                    return Err(
                        source.fail("missing ), unterminated subpattern", source.tell() - start)
                    );
                }
                if kind == '=' {
                    sequence.push(Node::Look {
                        ahead,
                        negate: false,
                        body,
                    });
                } else if body.is_empty() {
                    sequence.push(Node::Failure);
                } else {
                    sequence.push(Node::Look {
                        ahead,
                        negate: true,
                        body,
                    });
                }
                return Ok(());
            }
            '>' => {
                capture = false;
                atomic = true;
            }
            _ if flag_bit(character).is_some() || character == '-' => {
                match parse_flags(source, state, character)? {
                    None => {
                        if !first || !sequence.is_empty() {
                            return Err(source.fail(
                                "global flags not at the start of the expression",
                                source.tell() - start,
                            ));
                        }
                        return Ok(());
                    }
                    Some((added, removed)) => {
                        add = added;
                        del = removed;
                        capture = false;
                    }
                }
            }
            _ => {
                let message = format!("unknown extension ?{token}");
                return Err(source.fail(&message, token.chars().count() + 1));
            }
        }
    }
    let group = if capture {
        match state.open_group(name.as_deref()) {
            Ok(gid) => Some(gid),
            Err(message) => {
                let width = name.as_ref().map_or(0, |found| found.chars().count()) + 1;
                return Err(source.fail(&message, width));
            }
        }
    } else {
        None
    };
    verbose = (verbose || add & VERBOSE != 0) && del & VERBOSE == 0;
    let body = parse_sub(source, state, verbose, nested + 1)?;
    if !source.matches(")")? {
        return Err(source.fail("missing ), unterminated subpattern", source.tell() - start));
    }
    if let Some(gid) = group {
        state.close_group(gid);
    }
    if atomic {
        sequence.push(Node::Atomic(body));
    } else {
        sequence.push(Node::Group {
            index: group,
            add,
            del,
            body,
        });
    }
    Ok(())
}

fn inline_plain_groups(sequence: Vec<Node>) -> Vec<Node> {
    let mut out = Vec::with_capacity(sequence.len());
    for node in sequence {
        match node {
            Node::Group {
                index: None,
                add: 0,
                del: 0,
                body,
            } => out.extend(body),
            other => out.push(other),
        }
    }
    out
}

fn parse_sequence(
    source: &mut Source,
    state: &mut State,
    verbose: bool,
    nested: usize,
    first: bool,
) -> Result<Vec<Node>, String> {
    let mut sequence: Vec<Node> = Vec::new();
    let mut verbose = verbose;
    while let Some(this) = source.next.clone() {
        if this == "|" || this == ")" {
            break;
        }
        source.get()?;
        if verbose {
            if this.chars().count() == 1 && WHITESPACE.contains(&this) {
                continue;
            }
            if this == "#" {
                loop {
                    let found = source.get()?;
                    if found.is_none() || found.as_deref() == Some("\n") {
                        break;
                    }
                }
                continue;
            }
        }
        if this.starts_with('\\') {
            let escaped = escape_in_expression(source, &this, state)?;
            sequence.push(match escaped {
                Escaped::Literal(found) => Node::Literal(found),
                Escaped::Category(category) => Node::Category(category),
                Escaped::At(at) => Node::At(at),
                Escaped::BackRef(group) => Node::BackRef(group),
            });
        } else if !SPECIAL_CHARS.contains(&this) {
            sequence.push(Node::Literal(this.chars().next().unwrap_or('\0')));
        } else if this == "[" {
            let here = source.tell() - 1;
            sequence.push(parse_set(source, here)?);
        } else if REPEAT_CHARS.contains(&this) {
            parse_quantifier(source, &this, &mut sequence)?;
        } else if this == "." {
            sequence.push(Node::Any);
        } else if this == "(" {
            let flags_before = state.flags;
            parse_group(source, state, verbose, nested, &mut sequence, first)?;
            if state.flags != flags_before {
                verbose = state.flags & VERBOSE != 0;
            }
        } else if this == "^" {
            sequence.push(Node::At(At::Beginning));
        } else {
            sequence.push(Node::At(At::End));
        }
    }
    Ok(inline_plain_groups(sequence))
}

type Width = (usize, Option<usize>);

fn width_of(nodes: &[Node]) -> Width {
    nodes.iter().fold((0, Some(0)), |(low, high), node| {
        let (node_low, node_high) = node_width(node);
        (low + node_low, high.zip(node_high).map(|(a, b)| a + b))
    })
}

fn node_width(node: &Node) -> Width {
    match node {
        Node::Literal(_) | Node::Any | Node::Set { .. } | Node::Category(_) => (1, Some(1)),
        Node::At(_) | Node::Look { .. } | Node::Failure => (0, Some(0)),
        Node::BackRef(_) => (0, None),
        Node::Group { body, .. } | Node::Atomic(body) => width_of(body),
        Node::Alt(alternatives) => alternatives.iter().map(|a| width_of(a)).fold(
            (usize::MAX, Some(0)),
            |(low, high), (next_low, next_high)| {
                (
                    low.min(next_low),
                    high.zip(next_high).map(|(a, b)| a.max(b)),
                )
            },
        ),
        Node::Repeat { min, max, item, .. } => {
            let (low, high) = width_of(item);
            (
                low.saturating_mul(*min),
                max.zip(high)
                    .map(|(count, each)| count.saturating_mul(each)),
            )
        }
    }
}

fn check_lookbehinds(nodes: &[Node]) -> Result<(), String> {
    for node in nodes {
        match node {
            Node::Look { ahead, body, .. } => {
                if !ahead {
                    let (low, high) = width_of(body);
                    if Some(low) != high {
                        return Err("look-behind requires fixed-width pattern".to_string());
                    }
                }
                check_lookbehinds(body)?;
            }
            Node::Group { body, .. } | Node::Atomic(body) => check_lookbehinds(body)?,
            Node::Repeat { item, .. } => check_lookbehinds(item)?,
            Node::Alt(alternatives) => {
                for alternative in alternatives {
                    check_lookbehinds(alternative)?;
                }
            }
            _ => {}
        }
    }
    Ok(())
}

pub fn compile(pattern: &str) -> Result<Regex, String> {
    let mut source = Source::new(pattern)?;
    let mut state = State {
        flags: 0,
        group_names: Vec::new(),
        group_count: 1,
        open_groups: Vec::new(),
        lookbehind_groups: None,
    };
    let root = parse_sub(&mut source, &mut state, false, 0)?;
    if source.next.is_some() {
        return Err(source.fail("unbalanced parenthesis", 0));
    }
    check_lookbehinds(&root)?;
    if state.flags & ASCII != 0 && state.flags & UNICODE != 0 {
        return Err("ASCII and UNICODE flags are incompatible".to_string());
    }
    Ok(Regex {
        root,
        groups: state.group_count,
        flags: state.flags,
        names: state.group_names,
    })
}

type Continuation<'a> = &'a mut dyn FnMut(&mut Matcher, usize) -> bool;

struct Matcher<'a> {
    text: &'a [char],
    captures: Vec<Option<(usize, usize)>>,
}

fn is_word(character: char, flags: u8) -> bool {
    if flags & ASCII != 0 {
        return character.is_ascii_alphanumeric() || character == '_';
    }
    is_alphanumeric(character) || character == '_'
}

fn in_category(category: Category, character: char, flags: u8) -> bool {
    let digit = if flags & ASCII != 0 {
        character.is_ascii_digit()
    } else {
        decimal_value(character).is_some()
    };
    let space = if flags & ASCII != 0 {
        matches!(character, ' ' | '\t' | '\n' | '\r' | '\x0b' | '\x0c')
    } else {
        py::is_space(character)
    };
    match category {
        Category::Digit => digit,
        Category::NotDigit => !digit,
        Category::Space => space,
        Category::NotSpace => !space,
        Category::Word => is_word(character, flags),
        Category::NotWord => !is_word(character, flags),
    }
}

fn lower(character: char) -> char {
    let mut mapped = character.to_lowercase();
    match (mapped.next(), mapped.next()) {
        (Some(only), None) => only,
        _ => character,
    }
}

fn upper(character: char) -> char {
    let mut mapped = character.to_uppercase();
    match (mapped.next(), mapped.next()) {
        (Some(only), None) => only,
        _ => character,
    }
}

fn same_character(left: char, right: char, flags: u8) -> bool {
    left == right || (flags & IGNORE_CASE != 0 && lower(left) == lower(right))
}

fn set_holds(items: &[SetItem], character: char, flags: u8) -> bool {
    let holds = |candidate: char| {
        items.iter().any(|item| match item {
            SetItem::Char(found) => *found == candidate,
            SetItem::Range(low, high) => (*low..=*high).contains(&candidate),
            SetItem::Category(category) => in_category(*category, candidate, flags),
        })
    };
    holds(character)
        || (flags & IGNORE_CASE != 0 && (holds(lower(character)) || holds(upper(character))))
}

impl Matcher<'_> {
    fn at(&self, at: At, position: usize, flags: u8) -> bool {
        let length = self.text.len();
        let word = |index: Option<usize>| {
            index
                .and_then(|i| self.text.get(i))
                .is_some_and(|c| is_word(*c, flags))
        };
        match at {
            At::BeginningString => position == 0,
            At::EndString => position == length,
            At::Beginning => {
                position == 0 || (flags & MULTILINE != 0 && self.text[position - 1] == '\n')
            }
            At::End => {
                position == length
                    || (self.text[position] == '\n'
                        && (flags & MULTILINE != 0 || position + 1 == length))
            }
            At::Boundary | At::NonBoundary => {
                let before = word(position.checked_sub(1));
                let after = word(Some(position));
                let boundary = before != after && (length > 0);
                if at == At::Boundary {
                    boundary
                } else {
                    !boundary && length > 0
                }
            }
        }
    }

    fn sequence(&mut self, nodes: &[Node], position: usize, flags: u8, done: Continuation) -> bool {
        match nodes.split_first() {
            None => done(self, position),
            Some((head, rest)) => self.node(head, position, flags, &mut |matcher, next| {
                matcher.sequence(rest, next, flags, done)
            }),
        }
    }

    fn single_step(&self, node: &Node, position: usize, flags: u8) -> Option<bool> {
        let character = self.text.get(position).copied();
        let matched = match node {
            Node::Literal(wanted) => character.is_some_and(|c| same_character(*wanted, c, flags)),
            Node::Any => character.is_some_and(|c| flags & DOT_ALL != 0 || c != '\n'),
            Node::Set { negate, items } => {
                character.is_some_and(|c| set_holds(items, c, flags) != *negate)
            }
            Node::Category(category) => character.is_some_and(|c| in_category(*category, c, flags)),
            _ => return None,
        };
        Some(matched)
    }

    fn node(&mut self, node: &Node, position: usize, flags: u8, done: Continuation) -> bool {
        if let Some(matched) = self.single_step(node, position, flags) {
            return matched && done(self, position + 1);
        }
        match node {
            Node::At(at) => self.at(*at, position, flags) && done(self, position),
            Node::Failure => false,
            Node::Group {
                index,
                add,
                del,
                body,
            } => {
                let inner = (flags | add) & !del;
                match index {
                    None => self.sequence(body, position, inner, done),
                    Some(slot) => {
                        let slot = *slot;
                        self.sequence(body, position, inner, &mut |matcher, end| {
                            let old = matcher.captures[slot];
                            matcher.captures[slot] = Some((position, end));
                            if done(matcher, end) {
                                return true;
                            }
                            matcher.captures[slot] = old;
                            false
                        })
                    }
                }
            }
            Node::Alt(alternatives) => alternatives
                .iter()
                .any(|alternative| self.sequence(alternative, position, flags, done)),
            Node::Repeat {
                min,
                max,
                mode,
                item,
            } => {
                if *mode == Mode::Possessive {
                    let end = self.first_end(
                        &[Node::Repeat {
                            min: *min,
                            max: *max,
                            mode: Mode::Greedy,
                            item: item.clone(),
                        }],
                        position,
                        flags,
                    );
                    return end.is_some_and(|end| done(self, end));
                }
                self.repeat(item, (*min, *max, *mode), 0, position, flags, done)
            }
            Node::BackRef(group) => {
                let Some(Some((start, end))) = self.captures.get(*group).copied() else {
                    return false;
                };
                let length = end - start;
                if position + length > self.text.len() {
                    return false;
                }
                let same = (0..length)
                    .all(|i| same_character(self.text[start + i], self.text[position + i], flags));
                same && done(self, position + length)
            }
            Node::Look {
                ahead,
                negate,
                body,
            } => {
                let found = if *ahead {
                    self.first_end(body, position, flags).is_some()
                } else {
                    self.behind(body, position, flags)
                };
                found != *negate && done(self, position)
            }
            Node::Atomic(body) => {
                let end = self.first_end(body, position, flags);
                end.is_some_and(|end| done(self, end))
            }
            _ => false,
        }
    }

    fn first_end(&mut self, body: &[Node], position: usize, flags: u8) -> Option<usize> {
        let mut found = None;
        self.sequence(body, position, flags, &mut |_, end| {
            found = Some(end);
            true
        });
        found
    }

    fn behind(&mut self, body: &[Node], position: usize, flags: u8) -> bool {
        let (width, _) = width_of(body);
        let Some(start) = position.checked_sub(width) else {
            return false;
        };
        self.sequence(body, start, flags, &mut |_, end| end == position)
    }

    fn repeat(
        &mut self,
        item: &[Node],
        bounds: (usize, Option<usize>, Mode),
        count: usize,
        position: usize,
        flags: u8,
        done: Continuation,
    ) -> bool {
        let (min, max, mode) = bounds;
        let may_continue = max.is_none_or(|limit| count < limit);
        let more = |matcher: &mut Matcher, done: Continuation| {
            may_continue
                && matcher.sequence(item, position, flags, &mut |inner, next| {
                    if next == position && count >= min {
                        return false;
                    }
                    inner.repeat(item, bounds, count + 1, next, flags, done)
                })
        };
        if count < min {
            return more(self, done);
        }
        match mode {
            Mode::Lazy => done(self, position) || more(self, done),
            _ => more(self, done) || done(self, position),
        }
    }
}

impl Regex {
    pub fn match_at(&self, text: &[char], start: usize) -> Option<Match> {
        let mut matcher = Matcher {
            text,
            captures: vec![None; self.groups],
        };
        let mut finish = None;
        matcher.sequence(&self.root, start, self.flags, &mut |_, end| {
            finish = Some(end);
            true
        });
        matcher.captures[0] = Some((start, finish?));
        Some(Match {
            spans: matcher.captures,
            names: self.names.clone(),
        })
    }

    pub fn search(&self, text: &str) -> bool {
        let characters: Vec<char> = text.chars().collect();
        let mut matcher = Matcher {
            text: &characters,
            captures: vec![None; self.groups],
        };
        (0..=characters.len()).any(|start| {
            matcher.captures.iter_mut().for_each(|slot| *slot = None);
            matcher.sequence(&self.root, start, self.flags, &mut |_, _| true)
        })
    }
}

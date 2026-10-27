use crate::digits::decimal_value;
use crate::py::is_space;

#[derive(Clone, Debug)]
enum ClassItem {
    Single(char),
    Range(char, char),
    Shorthand(char),
}

#[derive(Clone, Debug)]
enum Node {
    Literal(char),
    Any,
    Class(Vec<ClassItem>, bool),
    Shorthand(char),
    Start,
    End,
    WordBoundary(bool),
    Group(Option<usize>, Vec<Vec<Node>>),
    Look(bool, Vec<Vec<Node>>),
    Repeat(Box<Node>, usize, Option<usize>, bool),
}

pub struct Regex {
    alternatives: Vec<Vec<Node>>,
    names: Vec<(String, usize)>,
    groups: usize,
}

pub struct Match {
    spans: Vec<Option<(usize, usize)>>,
    names: Vec<(String, usize)>,
}

type Spans = Vec<Option<(usize, usize)>>;

impl Match {
    pub fn end(&self) -> usize {
        self.spans[0].map_or(0, |(_, end)| end)
    }

    pub fn group(&self, index: usize) -> Option<(usize, usize)> {
        self.spans.get(index).copied().flatten()
    }

    pub fn named(&self, name: &str) -> Option<(usize, usize)> {
        let (_, index) = self.names.iter().find(|(known, _)| known == name)?;
        self.group(*index)
    }
}

pub fn escape(text: &str) -> String {
    text.chars()
        .flat_map(|c| {
            if c.is_alphanumeric() || c == '_' {
                vec![c]
            } else {
                vec!['\\', c]
            }
        })
        .collect()
}

impl Regex {
    pub fn new(pattern: &str) -> Result<Self, String> {
        let mut parser = Parser {
            chars: pattern.chars().collect(),
            at: 0,
            groups: 0,
            names: Vec::new(),
        };
        let alternatives = parser.alternatives()?;
        if parser.at != parser.chars.len() {
            return Err(format!("unbalanced parenthesis in {pattern}"));
        }
        Ok(Self {
            alternatives,
            names: parser.names,
            groups: parser.groups,
        })
    }

    pub fn match_at(&self, text: &[char], start: usize) -> Option<Match> {
        self.run(text, start)
    }

    fn run(&self, text: &[char], start: usize) -> Option<Match> {
        let mut spans: Spans = vec![None; self.groups + 1];
        let mut found: Option<Spans> = None;
        let matcher = Matcher { text };
        let root = vec![Node::Group(Some(0), self.alternatives.clone())];
        matcher.sequence(&root, start, &mut spans, &mut |_, spans| {
            found = Some(spans.clone());
            true
        });
        found.map(|spans| Match {
            spans,
            names: self.names.clone(),
        })
    }
}

struct Matcher<'a> {
    text: &'a [char],
}

type Continuation<'k> = dyn FnMut(usize, &mut Spans) -> bool + 'k;

impl Matcher<'_> {
    fn sequence(
        &self,
        nodes: &[Node],
        at: usize,
        spans: &mut Spans,
        next: &mut Continuation,
    ) -> bool {
        match nodes.split_first() {
            None => next(at, spans),
            Some((first, rest)) => self.node(first, at, spans, &mut |end, spans| {
                self.sequence(rest, end, spans, next)
            }),
        }
    }

    fn node(&self, node: &Node, at: usize, spans: &mut Spans, next: &mut Continuation) -> bool {
        let here = self.text.get(at).copied();
        match node {
            Node::Literal(c) => here == Some(*c) && next(at + 1, spans),
            Node::Any => here.is_some_and(|c| c != '\n') && next(at + 1, spans),
            Node::Class(items, negated) => {
                here.is_some_and(|c| class_holds(items, c) != *negated) && next(at + 1, spans)
            }
            Node::Shorthand(kind) => {
                here.is_some_and(|c| shorthand(*kind, c)) && next(at + 1, spans)
            }
            Node::Start => at == 0 && next(at, spans),
            Node::End => {
                (at == self.text.len() || (at + 1 == self.text.len() && self.text[at] == '\n'))
                    && next(at, spans)
            }
            Node::WordBoundary(wanted) => {
                let before = at > 0 && is_word(self.text[at - 1]);
                let after = here.is_some_and(is_word);
                (before != after) == *wanted && next(at, spans)
            }
            Node::Group(index, alternatives) => {
                let index = *index;
                alternatives.iter().any(|alternative| {
                    self.sequence(alternative, at, spans, &mut |end, spans| {
                        let Some(index) = index else {
                            return next(end, spans);
                        };
                        let before = spans[index];
                        spans[index] = Some((at, end));
                        next(end, spans) || {
                            spans[index] = before;
                            false
                        }
                    })
                })
            }
            Node::Look(negative, alternatives) => {
                let mut probe = spans.clone();
                let found = alternatives.iter().any(|alternative| {
                    self.sequence(alternative, at, &mut probe, &mut |_, _| true)
                });
                if found == *negative {
                    return false;
                }
                if !negative {
                    *spans = probe;
                }
                next(at, spans)
            }
            Node::Repeat(inner, min, max, lazy) => {
                self.repeat(inner, *min, *max, *lazy, 0, at, spans, next)
            }
        }
    }

    #[allow(clippy::too_many_arguments)]
    fn repeat(
        &self,
        inner: &Node,
        min: usize,
        max: Option<usize>,
        lazy: bool,
        count: usize,
        at: usize,
        spans: &mut Spans,
        next: &mut Continuation,
    ) -> bool {
        let may_stop = count >= min;
        let may_go = max.is_none_or(|max| count < max);
        if lazy && may_stop && next(at, spans) {
            return true;
        }
        if may_go
            && self.node(inner, at, spans, &mut |end, spans| {
                (end != at || count < min)
                    && self.repeat(inner, min, max, lazy, count + 1, end, spans, next)
            })
        {
            return true;
        }
        !lazy && may_stop && next(at, spans)
    }
}

pub fn is_word(c: char) -> bool {
    c == '_' || c.is_alphanumeric()
}

fn shorthand(kind: char, c: char) -> bool {
    match kind {
        's' => is_space(c),
        'S' => !is_space(c),
        'w' => is_word(c),
        'W' => !is_word(c),
        'd' => decimal_value(c).is_some(),
        'D' => decimal_value(c).is_none(),
        _ => false,
    }
}

fn class_holds(items: &[ClassItem], c: char) -> bool {
    items.iter().any(|item| match item {
        ClassItem::Single(single) => *single == c,
        ClassItem::Range(low, high) => (*low..=*high).contains(&c),
        ClassItem::Shorthand(kind) => shorthand(*kind, c),
    })
}

struct Parser {
    chars: Vec<char>,
    at: usize,
    groups: usize,
    names: Vec<(String, usize)>,
}

impl Parser {
    fn peek(&self) -> Option<char> {
        self.chars.get(self.at).copied()
    }

    fn eat(&mut self, text: &str) -> bool {
        let wanted: Vec<char> = text.chars().collect();
        if self.chars[self.at..].starts_with(&wanted) {
            self.at += wanted.len();
            return true;
        }
        false
    }

    fn alternatives(&mut self) -> Result<Vec<Vec<Node>>, String> {
        let mut alternatives = vec![self.sequence()?];
        while self.eat("|") {
            alternatives.push(self.sequence()?);
        }
        Ok(alternatives)
    }

    fn sequence(&mut self) -> Result<Vec<Node>, String> {
        let mut nodes = Vec::new();
        while let Some(c) = self.peek() {
            if c == '|' || c == ')' {
                break;
            }
            let atom = self.atom()?;
            nodes.push(self.quantified(atom)?);
        }
        Ok(nodes)
    }

    fn quantified(&mut self, atom: Node) -> Result<Node, String> {
        let (min, max) = match self.peek() {
            Some('*') => (0, None),
            Some('+') => (1, None),
            Some('?') => (0, Some(1)),
            Some('{') => match self.counted() {
                Some(bounds) => bounds,
                None => return Ok(atom),
            },
            _ => return Ok(atom),
        };
        self.at += 1;
        let lazy = self.eat("?");
        Ok(Node::Repeat(Box::new(atom), min, max, lazy))
    }

    fn counted(&mut self) -> Option<(usize, Option<usize>)> {
        let close = self.chars[self.at..].iter().position(|c| *c == '}')? + self.at;
        let inside: String = self.chars[self.at + 1..close].iter().collect();
        let (low, high) = match inside.split_once(',') {
            Some((low, high)) => (low.to_string(), Some(high.to_string())),
            None => (inside.clone(), None),
        };
        let min = if low.is_empty() { 0 } else { low.parse().ok()? };
        let max = match high {
            None => Some(min),
            Some(high) if high.is_empty() => None,
            Some(high) => Some(high.parse().ok()?),
        };
        self.at = close;
        Some((min, max))
    }

    fn atom(&mut self) -> Result<Node, String> {
        let c = self.peek().ok_or("unexpected end of pattern")?;
        self.at += 1;
        Ok(match c {
            '.' => Node::Any,
            '^' => Node::Start,
            '$' => Node::End,
            '[' => self.class()?,
            '(' => self.group()?,
            '\\' => self.escaped()?,
            other => Node::Literal(other),
        })
    }

    fn escaped(&mut self) -> Result<Node, String> {
        let c = self.peek().ok_or("trailing backslash")?;
        self.at += 1;
        Ok(match c {
            's' | 'S' | 'w' | 'W' | 'd' | 'D' => Node::Shorthand(c),
            'b' => Node::WordBoundary(true),
            'B' => Node::WordBoundary(false),
            'A' => Node::Start,
            'Z' => Node::Look(true, vec![vec![Node::Any]]),
            other => Node::Literal(control(other)),
        })
    }

    fn group(&mut self) -> Result<Node, String> {
        let node = if self.eat("?:") {
            Node::Group(None, self.alternatives()?)
        } else if self.eat("?!") {
            Node::Look(true, self.alternatives()?)
        } else if self.eat("?=") {
            Node::Look(false, self.alternatives()?)
        } else if self.eat("?P<") {
            let close = self.chars[self.at..]
                .iter()
                .position(|c| *c == '>')
                .ok_or("unterminated group name")?
                + self.at;
            let name: String = self.chars[self.at..close].iter().collect();
            self.at = close + 1;
            self.groups += 1;
            let index = self.groups;
            self.names.push((name, index));
            Node::Group(Some(index), self.alternatives()?)
        } else {
            self.groups += 1;
            let index = self.groups;
            Node::Group(Some(index), self.alternatives()?)
        };
        if !self.eat(")") {
            return Err("missing )".to_string());
        }
        Ok(node)
    }

    fn class(&mut self) -> Result<Node, String> {
        let negated = self.eat("^");
        let mut items = Vec::new();
        let mut first = true;
        loop {
            let c = self.peek().ok_or("unterminated character set")?;
            if c == ']' && !first {
                self.at += 1;
                break;
            }
            first = false;
            self.at += 1;
            let item = if c == '\\' {
                let escaped = self.peek().ok_or("trailing backslash")?;
                self.at += 1;
                if "sSwWdD".contains(escaped) {
                    ClassItem::Shorthand(escaped)
                } else {
                    ClassItem::Single(control(escaped))
                }
            } else {
                ClassItem::Single(c)
            };
            if let ClassItem::Single(low) = item
                && self.peek() == Some('-')
                && self.chars.get(self.at + 1).is_some_and(|next| *next != ']')
            {
                self.at += 1;
                let mut high = self.peek().ok_or("bad range")?;
                self.at += 1;
                if high == '\\' {
                    high = control(self.peek().ok_or("bad range")?);
                    self.at += 1;
                }
                items.push(ClassItem::Range(low, high));
                continue;
            }
            items.push(item);
        }
        Ok(Node::Class(items, negated))
    }
}

fn control(c: char) -> char {
    match c {
        'n' => '\n',
        't' => '\t',
        'r' => '\r',
        'f' => '\u{c}',
        'v' => '\u{b}',
        other => other,
    }
}

#[cfg(test)]
mod tests {
    use super::Regex;

    fn spans(pattern: &str, text: &str) -> Option<(usize, usize)> {
        let chars: Vec<char> = text.chars().collect();
        Regex::new(pattern)
            .unwrap()
            .match_at(&chars, 0)
            .map(|found| (0, found.end()))
    }

    #[test]
    fn quantifiers_backtrack_like_python() {
        let cases = [
            ("a*ab", "aaab", Some((0, 4))),
            ("a+?", "aaa", Some((0, 1))),
            ("a{2,3}", "aaaa", Some((0, 3))),
            ("a{2}", "a", None),
            ("(?:ab)+c", "ababc", Some((0, 5))),
            ("x(?::[^=]*?)?(?P<arrow>=)(?!=)", "x: T = 1", Some((0, 6))),
            ("x(?::[^=]*?)?(?P<arrow>=)(?!=)", "x == 1", None),
            ("[\\w.$<>\\[\\],?]+\\s+f\\(", "List<int> f(", Some((0, 12))),
            ("(?!return\\b)\\w+", "returned", Some((0, 8))),
            ("(?!return\\b)\\w+", "return x", None),
            ("a|ab|abc", "abc", Some((0, 1))),
            ("^\\s*def\\s+f", "  def f", Some((0, 7))),
            ("\\d+$", "12\n", Some((0, 2))),
        ];
        for (pattern, text, expected) in cases {
            assert_eq!(expected, spans(pattern, text), "{pattern} on {text:?}");
        }
    }

    #[test]
    fn named_groups_report_their_spans() {
        let regex = Regex::new("^(?P<indent>[ \\t]*)fn\\s+(?P<name>\\w+)").unwrap();
        let text: Vec<char> = "\t  fn main".chars().collect();
        let found = regex.match_at(&text, 0).unwrap();
        assert_eq!(
            (Some((0, 3)), Some((6, 10)), None),
            (
                found.named("indent"),
                found.named("name"),
                found.named("other")
            )
        );
    }
}

use std::collections::HashMap;

const CUTOFF: f64 = 0.6;
const AUTOJUNK_LENGTH: usize = 200;

struct Matcher {
    b: Vec<char>,
    b2j: HashMap<char, Vec<usize>>,
    counts: HashMap<char, usize>,
}

impl Matcher {
    fn new(word: &str) -> Self {
        let b: Vec<char> = word.chars().collect();
        let mut b2j: HashMap<char, Vec<usize>> = HashMap::new();
        for (index, character) in b.iter().enumerate() {
            b2j.entry(*character).or_default().push(index);
        }
        if b.len() >= AUTOJUNK_LENGTH {
            let popular = b.len() / 100 + 1;
            b2j.retain(|_, places| places.len() <= popular);
        }
        let mut counts = HashMap::new();
        for character in &b {
            *counts.entry(*character).or_insert(0) += 1;
        }
        Self { b, b2j, counts }
    }

    fn real_quick_ratio(&self, a: &[char]) -> f64 {
        ratio(a.len().min(self.b.len()), a.len() + self.b.len())
    }

    fn quick_ratio(&self, a: &[char]) -> f64 {
        let mut available: HashMap<char, isize> = HashMap::new();
        let mut matches = 0;
        for character in a {
            let left = available
                .entry(*character)
                .or_insert_with(|| self.counts.get(character).copied().unwrap_or(0) as isize);
            *left -= 1;
            if *left >= 0 {
                matches += 1;
            }
        }
        ratio(matches, a.len() + self.b.len())
    }

    fn ratio(&self, a: &[char]) -> f64 {
        let mut queue = vec![(0, a.len(), 0, self.b.len())];
        let mut matched = 0;
        while let Some((alo, ahi, blo, bhi)) = queue.pop() {
            let (i, j, k) = self.longest_match(a, alo, ahi, blo, bhi);
            if k == 0 {
                continue;
            }
            matched += k;
            if alo < i && blo < j {
                queue.push((alo, i, blo, j));
            }
            if i + k < ahi && j + k < bhi {
                queue.push((i + k, ahi, j + k, bhi));
            }
        }
        ratio(matched, a.len() + self.b.len())
    }

    fn longest_match(
        &self,
        a: &[char],
        alo: usize,
        ahi: usize,
        blo: usize,
        bhi: usize,
    ) -> (usize, usize, usize) {
        let (mut besti, mut bestj, mut bestsize) = (alo, blo, 0);
        let mut j2len: HashMap<usize, usize> = HashMap::new();
        for (i, character) in a.iter().enumerate().take(ahi).skip(alo) {
            let mut next: HashMap<usize, usize> = HashMap::new();
            for &j in self.b2j.get(character).map_or(&[][..], Vec::as_slice) {
                if j < blo {
                    continue;
                }
                if j >= bhi {
                    break;
                }
                let k = j
                    .checked_sub(1)
                    .and_then(|before| j2len.get(&before))
                    .copied()
                    .unwrap_or(0)
                    + 1;
                next.insert(j, k);
                if k > bestsize {
                    (besti, bestj, bestsize) = (i + 1 - k, j + 1 - k, k);
                }
            }
            j2len = next;
        }
        while besti > alo && bestj > blo && a[besti - 1] == self.b[bestj - 1] {
            (besti, bestj, bestsize) = (besti - 1, bestj - 1, bestsize + 1);
        }
        while besti + bestsize < ahi
            && bestj + bestsize < bhi
            && a[besti + bestsize] == self.b[bestj + bestsize]
        {
            bestsize += 1;
        }
        (besti, bestj, bestsize)
    }
}

fn ratio(matches: usize, length: usize) -> f64 {
    if length == 0 {
        1.0
    } else {
        2.0 * matches as f64 / length as f64
    }
}

pub fn close_matches<'a>(word: &str, possibilities: &[&'a str], limit: usize) -> Vec<&'a str> {
    let matcher = Matcher::new(word);
    let mut scored: Vec<(f64, &str)> = possibilities
        .iter()
        .filter_map(|candidate| {
            let a: Vec<char> = candidate.chars().collect();
            let close = matcher.real_quick_ratio(&a) >= CUTOFF
                && matcher.quick_ratio(&a) >= CUTOFF
                && matcher.ratio(&a) >= CUTOFF;
            close.then(|| (matcher.ratio(&a), *candidate))
        })
        .collect();
    scored.sort_by(|left, right| right.0.total_cmp(&left.0).then_with(|| right.1.cmp(left.1)));
    scored
        .into_iter()
        .take(limit)
        .map(|(_, candidate)| candidate)
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn close_matches_agree_with_the_documented_python_example() {
        let words = ["ape", "apple", "peach", "puppy"];
        assert_eq!(close_matches("appel", &words, 3), vec!["apple", "ape"]);
    }
}

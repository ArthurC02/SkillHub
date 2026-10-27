use crate::unicode_data::{
    ALPHANUMERIC, CASEFOLD, COMBINING_CLASS, COMPAT_DECOMPOSITION, COMPOSITION,
};

const HANGUL_BASE: u32 = 0xAC00;
const LEAD_BASE: u32 = 0x1100;
const VOWEL_BASE: u32 = 0x1161;
const TRAIL_BASE: u32 = 0x11A7;
const VOWEL_COUNT: u32 = 21;
const TRAIL_COUNT: u32 = 28;
const LEAD_COUNT: u32 = 19;
const SYLLABLE_COUNT: u32 = LEAD_COUNT * VOWEL_COUNT * TRAIL_COUNT;

fn mapped(table: &'static [(u32, &'static str)], character: char) -> Option<&'static str> {
    table
        .binary_search_by_key(&(character as u32), |(code, _)| *code)
        .ok()
        .map(|index| table[index].1)
}

pub fn casefold(text: &str) -> String {
    text.chars()
        .fold(String::with_capacity(text.len()), |mut out, character| {
            match mapped(CASEFOLD, character) {
                Some(folded) => out.push_str(folded),
                None => out.push(character),
            }
            out
        })
}

pub fn is_alphanumeric(character: char) -> bool {
    let code = character as u32;
    ALPHANUMERIC
        .binary_search_by(|(low, high)| {
            if *high < code {
                std::cmp::Ordering::Less
            } else if *low > code {
                std::cmp::Ordering::Greater
            } else {
                std::cmp::Ordering::Equal
            }
        })
        .is_ok()
}

fn combining_class(character: char) -> u8 {
    COMBINING_CLASS
        .binary_search_by_key(&(character as u32), |(code, _)| *code)
        .map_or(0, |index| COMBINING_CLASS[index].1)
}

fn decompose(character: char, out: &mut Vec<char>) {
    let offset = (character as u32).wrapping_sub(HANGUL_BASE);
    if offset < SYLLABLE_COUNT {
        let lead = LEAD_BASE + offset / (VOWEL_COUNT * TRAIL_COUNT);
        let vowel = VOWEL_BASE + (offset % (VOWEL_COUNT * TRAIL_COUNT)) / TRAIL_COUNT;
        let trail = TRAIL_BASE + offset % TRAIL_COUNT;
        out.extend([lead, vowel].into_iter().filter_map(char::from_u32));
        if trail != TRAIL_BASE {
            out.extend(char::from_u32(trail));
        }
        return;
    }
    match mapped(COMPAT_DECOMPOSITION, character) {
        Some(parts) => out.extend(parts.chars()),
        None => out.push(character),
    }
}

fn compose_pair(first: char, second: char) -> Option<char> {
    let (first, second) = (first as u32, second as u32);
    let lead = first.wrapping_sub(LEAD_BASE);
    let vowel = second.wrapping_sub(VOWEL_BASE);
    if lead < LEAD_COUNT && vowel < VOWEL_COUNT {
        return char::from_u32(HANGUL_BASE + (lead * VOWEL_COUNT + vowel) * TRAIL_COUNT);
    }
    let syllable = first.wrapping_sub(HANGUL_BASE);
    let trail = second.wrapping_sub(TRAIL_BASE);
    if syllable < SYLLABLE_COUNT && syllable % TRAIL_COUNT == 0 && (1..TRAIL_COUNT).contains(&trail)
    {
        return char::from_u32(first + trail);
    }
    COMPOSITION
        .binary_search_by_key(&(first, second), |(a, b, _)| (*a, *b))
        .ok()
        .and_then(|index| char::from_u32(COMPOSITION[index].2))
}

fn reorder(characters: &mut [char]) {
    let mut start = 0;
    while start < characters.len() {
        if combining_class(characters[start]) == 0 {
            start += 1;
            continue;
        }
        let end = characters[start..]
            .iter()
            .position(|character| combining_class(*character) == 0)
            .map_or(characters.len(), |offset| start + offset);
        characters[start..end].sort_by_key(|character| combining_class(*character));
        start = end;
    }
}

pub fn nfkc(text: &str) -> String {
    let mut decomposed = Vec::with_capacity(text.len());
    for character in text.chars() {
        decompose(character, &mut decomposed);
    }
    reorder(&mut decomposed);
    let Some(&first) = decomposed.first() else {
        return String::new();
    };
    let mut composed = vec![first];
    let mut starter = 0;
    let mut last_class = match combining_class(first) {
        0 => 0,
        _ => 256,
    };
    for &character in &decomposed[1..] {
        let class = u16::from(combining_class(character));
        let blocked = last_class != 0 && last_class >= class;
        if !blocked && let Some(joined) = compose_pair(composed[starter], character) {
            composed[starter] = joined;
            continue;
        }
        if class == 0 {
            starter = composed.len();
        }
        last_class = class;
        composed.push(character);
    }
    composed.into_iter().collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nfkc_folds_compatibility_forms_and_composes_marks() {
        assert_eq!(nfkc("ﬁ①Ａ"), "fi1A");
        assert_eq!(nfkc("e\u{301}"), "\u{e9}");
        assert_eq!(nfkc("\u{1100}\u{1161}\u{11A8}"), "\u{AC01}");
        assert_eq!(nfkc("\u{1E9B}\u{323}"), "\u{1E69}");
        assert_eq!(nfkc("\u{301}a"), "\u{301}a");
    }

    #[test]
    fn casefold_expands_and_alphanumeric_reads_every_script() {
        assert_eq!(casefold("Straße ΣΑΣ"), "strasse σασ");
        assert!(is_alphanumeric('名') && is_alphanumeric('٣') && !is_alphanumeric('_'));
    }
}

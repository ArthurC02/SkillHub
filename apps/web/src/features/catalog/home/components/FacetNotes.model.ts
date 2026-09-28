export type FacetNote<Row> = {
  key: string;
  label: string;
  note: (row: Row) => string | undefined;
  by?: (row: Row) => string;
};

export type LiftedNotes = Record<string, boolean>;

// A combined line would credit the same word with two different notes if a
// `by` word appeared under more than one note text.
function eachWordMapsToOneText(byNote: Map<string, Set<string>>): boolean {
  const claimed = new Map<string, string>();
  for (const [text, words] of byNote) {
    for (const word of words) {
      const owner = claimed.get(word);
      if (owner !== undefined && owner !== text) return false;
      claimed.set(word, text);
    }
  }
  return true;
}

export function facetNoteLines<Row>(
  rows: Row[],
  facets: Array<FacetNote<Row>>,
): Array<{ key: string; text: string }> {
  if (rows.length < 2) return [];
  const lines: Array<{ key: string; text: string }> = [];
  for (const { key, label, note, by } of facets) {
    const byNote = new Map<string, Set<string>>();
    let complete = true;
    for (const row of rows) {
      const text = note(row);
      if (!text) {
        complete = false;
        break;
      }
      const words = byNote.get(text) ?? new Set<string>();
      if (by) words.add(by(row));
      byNote.set(text, words);
    }
    if (!complete) continue;
    if (byNote.size === 1) {
      lines.push({ key, text: `${label}：${[...byNote.keys()][0]}` });
      continue;
    }
    if (!by) continue;
    if (!eachWordMapsToOneText(byNote)) continue;
    for (const [text, words] of byNote) {
      lines.push({ key, text: `${label}「${[...words].join("、")}」：${text}` });
    }
  }
  return lines;
}

export function liftedNotes<Row>(rows: Row[], facets: Array<FacetNote<Row>>): LiftedNotes {
  const lifted: LiftedNotes = {};
  for (const { key } of facetNoteLines(rows, facets)) lifted[key] = true;
  return lifted;
}

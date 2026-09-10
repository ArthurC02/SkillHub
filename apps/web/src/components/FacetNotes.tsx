import type { ReactElement } from "react";

export type FacetNote<Row> = {
  key: string;
  label: string;
  note: (row: Row) => string | undefined;
  by?: (row: Row) => string;
};

export type LiftedNotes = Record<string, boolean>;

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
    // Each `by` word must map to exactly one note text, or a combined line
    // would credit the same word with two different notes.
    const claimed = new Map<string, string>();
    let unambiguous = true;
    for (const [text, words] of byNote) {
      for (const word of words) {
        const owner = claimed.get(word);
        if (owner !== undefined && owner !== text) {
          unambiguous = false;
          break;
        }
        claimed.set(word, text);
      }
      if (!unambiguous) break;
    }
    if (!unambiguous) continue;
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

export function FacetNotes<Row>({
  rows,
  facets,
}: {
  rows: Row[];
  facets: Array<FacetNote<Row>>;
}): ReactElement {
  return (
    <>
      {facetNoteLines(rows, facets).map(({ text }) => (
        <p key={text} className="note">
          {text}
        </p>
      ))}
    </>
  );
}

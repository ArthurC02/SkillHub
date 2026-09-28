import { expect, test } from "vitest";
import { facetNoteLines, liftedNotes, type FacetNote } from "./FacetNotes.model";

type Row = { id: string; note: string };

const facet: FacetNote<Row> = {
  key: "risk",
  label: "風險",
  note: (row) => row.note,
  by: (row) => row.id,
};

test("facetNoteLines lifts a shared note to one line when every row agrees", () => {
  const rows: Row[] = [
    { id: "a", note: "已掃描" },
    { id: "b", note: "已掃描" },
  ];

  expect(facetNoteLines(rows, [facet])).toEqual([{ key: "risk", text: "風險：已掃描" }]);
  expect(liftedNotes(rows, [facet])).toEqual({ risk: true });
});

test("facetNoteLines splits per distinct note when each row's id maps to exactly one note", () => {
  const rows: Row[] = [
    { id: "a", note: "已掃描" },
    { id: "b", note: "從未掃描" },
  ];

  expect(facetNoteLines(rows, [facet])).toEqual([
    { key: "risk", text: "風險「a」：已掃描" },
    { key: "risk", text: "風險「b」：從未掃描" },
  ]);
});

test("facetNoteLines drops the facet when one id is claimed by two different notes", () => {
  const ambiguous: FacetNote<Row> = {
    key: "risk",
    label: "風險",
    note: (row) => row.note,
    by: () => "same-id",
  };
  const rows: Row[] = [
    { id: "a", note: "已掃描" },
    { id: "b", note: "從未掃描" },
  ];

  expect(facetNoteLines(rows, [ambiguous])).toEqual([]);
  expect(liftedNotes(rows, [ambiguous])).toEqual({});
});

import {
  FacetNotes as FacetNoteLines,
  liftedNotes as liftedNotesOf,
  type FacetNote,
  type LiftedNotes,
} from "./FacetNotes";
import type { PublicSearchResult } from "../../../../core/api/types";

const FACET_NOTES: Array<FacetNote<PublicSearchResult>> = [
  { key: "tier", label: "來源層級", note: (hit) => hit.tier.note, by: (hit) => hit.tier.label },
  {
    key: "category",
    label: "類別",
    note: (hit) => hit.category.note,
    by: (hit) => hit.category.label,
  },
  { key: "compatibility", label: "相容狀態", note: (hit) => hit.compatibility.note },
  { key: "risk", label: "風險提示", note: (hit) => hit.risk.note },
];

export function liftedNotes(hits: PublicSearchResult[]): LiftedNotes {
  return liftedNotesOf(hits, FACET_NOTES);
}

export function SearchFacetNotes({ hits }: { hits: PublicSearchResult[] }) {
  return <FacetNoteLines rows={hits} facets={FACET_NOTES} />;
}

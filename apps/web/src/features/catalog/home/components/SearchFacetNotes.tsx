import { FacetNotes } from "./FacetNotes";
import { FACET_NOTES } from "./SearchFacetNotes.model";
import type { PublicSearchResult } from "../../../../core/api/types";

export function SearchFacetNotes({ hits }: { hits: PublicSearchResult[] }) {
  return <FacetNotes rows={hits} facets={FACET_NOTES} />;
}

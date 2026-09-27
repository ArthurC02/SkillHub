import type { PublicSearchResult } from "../../../../core/api/types";
import { RankingExplainer } from "./RankingExplainer";
import { CompareBar } from "./CompareBar";
import { MarkerLegend } from "./MarkerLegend";
import { SearchFacetNotes } from "./SearchFacetNotes";
import { liftedNotes } from "./SearchFacetNotes.model";
import { SearchResultRow } from "./SearchResultRow";

export function SearchResultsList({
  query,
  results,
  selected,
  atLimit,
  onToggle,
}: {
  query: string;
  results: PublicSearchResult[];
  selected: string[];
  atLimit: boolean;
  onToggle: (skillId: string) => void;
}) {
  return (
    <>
      <RankingExplainer />
      <CompareBar selected={selected} />
      <h2 id="results-heading">符合「{query}」的 Skill</h2>
      <p role="status" className="note">
        找到 {results.length} 個 Skill。
      </p>
      <MarkerLegend />
      <SearchFacetNotes hits={results} />
      <ul className="search-results" aria-labelledby="results-heading">
        {results.map((hit) => (
          <SearchResultRow
            key={hit.skill_id}
            hit={hit}
            checked={selected.includes(hit.skill_id)}
            atLimit={atLimit}
            onToggle={onToggle}
            lifted={liftedNotes(results)}
          />
        ))}
      </ul>
    </>
  );
}

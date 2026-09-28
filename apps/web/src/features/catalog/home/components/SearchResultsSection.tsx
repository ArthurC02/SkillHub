import type { PublicSearchResponse, SearchCorrection } from "../../../../core/api/types";
import { IntentInterpretation } from "./IntentInterpretation";
import { NoResultsPanel } from "./NoResultsPanel";
import { SearchResultsList } from "./SearchResultsList";
import { MAX_COMPARE } from "../../../skill";

export function SearchResultsSection({
  data,
  selected,
  loggedIn,
  generateExposed,
  onCorrect,
  onClearFilters,
  onToggle,
}: {
  data: PublicSearchResponse;
  selected: string[];
  loggedIn: boolean;
  generateExposed: boolean;
  onCorrect: (correction: SearchCorrection) => void;
  onClearFilters: () => void;
  onToggle: (skillId: string) => void;
}) {
  return (
    <>
      <p>
        查詢：<q>{data.query}</q>
      </p>

      {data.interpretation && data.interpretation.status !== "skipped" && (
        <IntentInterpretation
          key={`${data.query}:${JSON.stringify(data.interpretation)}`}
          interpretation={data.interpretation}
          onCorrect={onCorrect}
        />
      )}

      {data.degraded && (
        <p className="notice" role="status">
          目前只用關鍵字比對搜尋，跨語言與語意相近的結果會找不到，召回率明顯較低。
        </p>
      )}
      {data.partial_index && (
        <p className="notice" role="status">
          部分 Skill 尚未建立語意索引，只能靠關鍵字命中，沒有相似度可顯示，並排在最後。
        </p>
      )}
      {data.truncated && (
        <p className="notice" role="status">
          符合的 Skill 共 {data.total} 個，這裡只列出最接近的 {data.results.length} 個。
          目前沒有翻頁；縮小任務描述或加上篩選條件會讓排序更貼近你要的。
        </p>
      )}

      {data.filtered_out && (
        <div>
          <p>有符合這個任務的 Skill，但全部被目前的篩選條件排除了。</p>
          <p>放寬或清除下方的篩選條件即可看到它們。</p>
          <button type="button" onClick={onClearFilters}>
            清除所有篩選
          </button>
        </div>
      )}

      {data.no_results && (
        <NoResultsPanel
          query={data.query}
          degraded={data.degraded}
          querySuggestion={data.query_suggestion}
          loggedIn={loggedIn}
          generateExposed={generateExposed}
        />
      )}

      {data.results.length > 0 && (
        <SearchResultsList
          query={data.query}
          results={data.results}
          selected={selected}
          atLimit={selected.length >= MAX_COMPARE}
          onToggle={onToggle}
        />
      )}
    </>
  );
}

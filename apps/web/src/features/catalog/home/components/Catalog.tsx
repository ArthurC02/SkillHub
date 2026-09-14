import { Link } from "@tanstack/react-router";
import { useCatalog } from "../../../skill";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Loading } from "../../../../shared/ui/Loading";
import { MAX_COMPARE } from "../../../skill";
import type { PublicSearchResult } from "../../../../core/api/types";
import { CompareBar } from "./CompareBar";
import { liftedNotes, SearchFacetNotes } from "./SearchFacetNotes";
import { MarkerLegend } from "./MarkerLegend";
import { SearchResultRow } from "./SearchResultRow";

export function Catalog({
  query,
  selected,
  onToggle,
  narrowing,
  tierFiltered,
}: {
  query: ReturnType<typeof useCatalog>;
  selected: string[];
  onToggle: (skillId: string) => void;
  narrowing: boolean;
  tierFiltered: boolean;
}) {
  if (query.isPending) return <Loading what="目錄" />;
  if (query.error) return <ReadFailure error={query.error} what="目錄" />;
  if (!query.data) return null;

  const { results, total, truncated } = query.data;

  const curated = results.filter((hit) => hit.tier.value === "curated");
  const rest = results.filter((hit) => hit.tier.value !== "curated");
  const shelved = !tierFiltered && curated.length > 0 && rest.length > 0;

  const row = (hit: PublicSearchResult) => (
    <SearchResultRow
      key={hit.skill_id}
      hit={hit}
      checked={selected.includes(hit.skill_id)}
      atLimit={selected.length >= MAX_COMPARE}
      onToggle={onToggle}
      rankNoteInList={false}
      lifted={liftedNotes(results)}
    />
  );

  return (
    <section aria-labelledby="catalog-heading">
      <h2 id="catalog-heading">目錄裡有什麼</h2>
      {results.length === 0 ? (
        narrowing ? (
          <p>沒有 Skill 符合目前的篩選條件；這不是讀取失敗。清掉篩選條件可查看完整目錄。</p>
        ) : (
          <div className="empty-catalog">
            <p className="empty-catalog-lede">目錄裡還沒有任何東西。</p>
            <p>
              這不是讀取失敗，也不是你沒有權限——是
              <strong>這個部署還沒有匯入過任何 Skill</strong>。
            </p>
            <p>
              <Link className="empty-catalog-cta" to="/workspace/import">
                匯入第一個 Skill
              </Link>
            </p>
            <p className="note" data-role="teaching">
              匯入是貼一個 GitHub URL、或上傳一個 zip。平台會做規格驗證與靜態掃描，
              匯入期間不執行套件裡的任何 Script。
            </p>
          </div>
        )
      ) : (
        <>
          <p className="note">{results[0]?.rank_note ?? "未提供目錄排序說明。"}</p>
          <CompareBar selected={selected} />
          <p role="status" className="note">
            {truncated
              ? `目錄共 ${total} 個 Skill，這裡列出 ${results.length} 個。目前沒有翻頁；用上面的搜尋或篩選縮小範圍。`
              : `目錄共 ${total} 個 Skill，全部列在下面。`}
          </p>
          <MarkerLegend />
          <SearchFacetNotes hits={results} />
          {shelved ? (
            <>
              <section className="curated-shelf" aria-labelledby="curated-heading">
                <h3 id="curated-heading">精選（{curated.length}）</h3>
                <p className="note">
                  這 {curated.length} 個 Skill 由我們自己逐份讀過：這一版通過九項人工檢視——來源
                  可追溯、License 實查、規格驗證、Script 逐行審閱、無疑似 Secret、白話摘要，以及至少
                  一次平台基準試跑符合。這不是安全保證，也不是推薦：平台不曾執行它們的程式碼來判斷
                  行為。審查綁在這一版的位元組上——出了新版本而審查還沒跟上，它會自動掉到下面那一段，
                  不需要任何人操作。下面 {rest.length} 個是「已索引」：目前這一版沒有帶著人工審查
                  結論，不是從沒被審過。
                </p>
                <ul className="search-results" aria-labelledby="curated-heading">
                  {curated.map(row)}
                </ul>
              </section>
              <h3 id="rest-heading">其餘目錄（{rest.length}）</h3>
              <ul className="search-results" aria-labelledby="rest-heading">
                {rest.map(row)}
              </ul>
            </>
          ) : (
            <ul className="search-results" aria-label="目錄">
              {results.map(row)}
            </ul>
          )}
        </>
      )}
    </section>
  );
}

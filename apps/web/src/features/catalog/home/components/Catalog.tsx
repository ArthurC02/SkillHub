import { Link } from "@tanstack/react-router";
import { useCatalog } from "../../../skill";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Loading } from "../../../../shared/ui/Loading";
import { MAX_COMPARE } from "../../../skill";
import type { PublicSearchResult } from "../../../../core/api/types";
import { CompareBar } from "./CompareBar";
import { SearchFacetNotes } from "./SearchFacetNotes";
import { liftedNotes } from "./SearchFacetNotes.model";
import { MarkerLegend, MarkerWarning } from "./MarkerLegend";
import { SearchResultRow } from "./SearchResultRow";
import "./Catalog.css";

function CatalogHeader({
  results,
  total,
  truncated,
}: {
  results: PublicSearchResult[];
  total: number;
  truncated: boolean;
}) {
  return (
    <div className="catalog-heading-row">
      <div>
        <p className="catalog-kicker">瀏覽 Catalog</p>
        <h2 id="catalog-heading">Skill 探索畫廊</h2>
      </div>
      {results.length > 0 && (
        <div className="catalog-heading-meta">
          <p role="status" className="catalog-total">
            {truncated ? `共 ${total} 個，顯示前 ${results.length} 個` : `共 ${total} 個 Skill`}
          </p>
          <p className="catalog-order">{results[0]?.rank_note ?? "未提供目錄排序說明。"}</p>
        </div>
      )}
    </div>
  );
}

function CatalogEmptyState({ narrowing }: { narrowing: boolean }) {
  if (narrowing) {
    return <p>沒有 Skill 符合目前的篩選條件；這不是讀取失敗。清掉篩選條件可查看完整目錄。</p>;
  }

  return (
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
  );
}

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
      compactFacets
      lifted={liftedNotes(results)}
    />
  );

  return (
    <section className="catalog-showcase" aria-labelledby="catalog-heading">
      <CatalogHeader results={results} total={total} truncated={truncated} />
      {results.length === 0 ? (
        <CatalogEmptyState narrowing={narrowing} />
      ) : (
        <>
          <CompareBar selected={selected} />
          {truncated && (
            <p className="note">
              目錄共 {total} 個 Skill，這裡列出 {results.length}{" "}
              個。目前沒有翻頁；用上面的搜尋或篩選縮小範圍。
            </p>
          )}
          {shelved ? (
            <>
              <section className="curated-shelf" aria-labelledby="curated-heading">
                <div className="catalog-shelf-heading">
                  <h3 id="curated-heading">精選 Skill（{curated.length}）</h3>
                </div>
                <ul className="search-results catalog-gallery" aria-labelledby="curated-heading">
                  {curated.map(row)}
                </ul>
                <p className="note catalog-shelf-note">
                  這一版由我們逐份讀過，通過九項人工檢視：來源可追溯、License 實查、規格驗證、
                  Script 逐行審閱、無疑似 Secret、白話摘要、至少一次平台基準試跑符合。
                  這不是安全保證，也不是推薦；平台未執行套件程式碼來判斷行為。審查綁在這一版的
                  位元組上；更新後若未重審，就會掉回「已索引」。
                </p>
              </section>
              <section className="catalog-shelf" aria-labelledby="rest-heading">
                <div className="catalog-shelf-heading">
                  <h3 id="rest-heading">近期收錄（{rest.length}）</h3>
                </div>
                <ul className="search-results catalog-gallery" aria-labelledby="rest-heading">
                  {rest.map(row)}
                </ul>
                <p className="note catalog-shelf-note">
                  「已索引」表示目前這一版沒有帶著人工審查結論，不是從沒被審過。
                </p>
              </section>
            </>
          ) : (
            <ul className="search-results catalog-gallery" aria-label="目錄">
              {results.map(row)}
            </ul>
          )}
          <div className="catalog-supporting-notes">
            <MarkerWarning />
            <SearchFacetNotes hits={results} />
            <MarkerLegend />
          </div>
          <p className="catalog-create-note">
            已經有自己的 Skill？
            <Link to="/library" hash="create">
              前往資產庫匯入或建立
            </Link>
            。
          </p>
        </>
      )}
    </section>
  );
}

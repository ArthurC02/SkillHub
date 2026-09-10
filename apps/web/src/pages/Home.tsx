import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useEffect, useState, type FormEvent } from "react";
import { useCatalog, useCatalogTotal, useSkillSearch } from "../api/skills";
import { ReadFailure } from "../components/LoginRequired";
import { useGenerateEntryPoint } from "../api/generate";
import { useMe } from "../api/me";
import { GenerateSkill } from "../components/GenerateSkill";
import { Loading } from "../components/Loading";
import { LabelledBadge } from "../components/LabelledBadge";
import { RiskSummary } from "../components/RiskIndicator";
import {
  FacetNotes as FacetNoteLines,
  liftedNotes as liftedNotesOf,
  type FacetNote,
  type LiftedNotes,
} from "../components/FacetNotes";
import { SignInAction } from "../components/SignIn";
import { Timestamp } from "../components/Timestamp";
import { MAX_COMPARE } from "./Compare";
import type { HomeSearch } from "../router";
import type { PublicSearchResult, SearchFilters, SkillCategory } from "../api/types";

function parseSelection(value: string | undefined): string[] {
  return value ? value.split(",").filter(Boolean).slice(0, MAX_COMPARE) : [];
}

const SEARCH_MAX_QUERY = 2000;
const runes = (s: string) => [...s].length;

export function Home() {
  const search = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });
  const [draft, setDraft] = useState(search.q ?? "");
  useEffect(() => setDraft(search.q ?? ""), [search.q]);
  const [queryError, setQueryError] = useState("");
  const selected = parseSelection(search.compare);
  const generateExposed = useGenerateEntryPoint();
  const loggedIn = !!useMe().data;

  const filters: SearchFilters = {
    script: search.script,
    validation: search.validation,
    agent: search.agent,
    tier: search.tier,
    category: search.category,
  };
  const { data, isFetching, error } = useSkillSearch(
    search.q ?? "",
    filters,
    search.q !== undefined,
  );
  const browsing = search.q === undefined;
  const catalog = useCatalog(filters, browsing);

  function submitSearch(next: Partial<typeof search>) {
    void navigate({
      search: (prev) => ({ ...prev, compare: undefined, ...next }),
      replace: true,
    });
  }

  function clearFilters() {
    void navigate({ search: { q: search.q }, replace: true });
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = draft.trim();
    const count = runes(trimmed);
    if (count > SEARCH_MAX_QUERY) {
      setQueryError(`搜尋文字最多 ${SEARCH_MAX_QUERY} 字，目前 ${count} 字。`);
      return;
    }
    setQueryError("");
    submitSearch({ q: trimmed || undefined });
  }

  function toggleSelected(skillId: string) {
    void navigate({
      search: (prev: HomeSearch) => {
        const current = parseSelection(prev.compare);
        const next = current.includes(skillId)
          ? current.filter((id) => id !== skillId)
          : current.length >= MAX_COMPARE
            ? current
            : [...current, skillId];
        return { ...prev, compare: next.length ? next.join(",") : undefined };
      },
      replace: true,
    });
  }

  return (
    <section className="home">
      <div className="hero">
        <h1>用一句話描述你的任務</h1>
        <form onSubmit={handleSubmit}>
          <input
            type="text"
            value={draft}
            onChange={(event) => {
              setDraft(event.target.value);
              if (queryError) setQueryError("");
            }}
            placeholder="在目錄裡找一個 Skill，例如：把這份 PDF 整理成摘要"
            aria-label="任務描述"
          />
          {queryError && <p role="alert">{queryError}</p>}
          <button type="submit" className="action">
            搜尋
          </button>
          <Link className="hero-create" to="/workspace/skills" hash="create">
            自己做一個 Skill
          </Link>
        </form>
      </div>

      <CategoryNav filters={filters} browsing={browsing} />

      <FilterBar filters={filters} onChange={(next) => submitSearch(next)} />

      {browsing && (
        <Catalog
          query={catalog}
          selected={selected}
          onToggle={toggleSelected}
          narrowing={Object.values(filters).some((value) => value !== undefined)}
          tierFiltered={filters.tier !== undefined}
        />
      )}

      {!browsing && isFetching && <p role="status">搜尋中…</p>}
      {!browsing && <ReadFailure error={error} what="搜尋結果" />}

      {!browsing && data && (
        <>
          <p>
            查詢：<q>{data.query}</q>
          </p>

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
              <button type="button" onClick={clearFilters}>
                清除所有篩選
              </button>
            </div>
          )}

          {data.no_results && (
            <div>
              <p>沒有夠接近的 Skill。</p>
              {data.degraded ? (
                <p>
                  而且這次搜尋只用了關鍵字比對，語意相近與跨語言的結果找不出來——現在找不到不代表
                  目錄裡沒有。換個說法幫不上忙；直接看目錄，或稍後再搜尋一次。
                </p>
              ) : (
                data.query_suggestion && <p>{data.query_suggestion}</p>
              )}
              <p className="note">
                <Link to="/" search={{}}>
                  看看目錄裡有什麼
                </Link>
                ——不帶任何查詢，列出這個部署收錄的全部 Skill。
              </p>
              <div className="note">
                {loggedIn ? (
                  <>
                    手上已經有一個 Skill 套件的話，也可以
                    <Link to="/workspace/import">直接匯入它</Link>。
                  </>
                ) : (
                  <>
                    手上已經有一個 Skill 套件的話，登入後可以把它匯入你自己的工作區。{" "}
                    <SignInAction />
                  </>
                )}
              </div>
              {generateExposed && <GenerateSkill initialTask={data.query} />}
            </div>
          )}

          {data.results.length > 0 && (
            <>
              <RankingExplainer />
              <CompareBar selected={selected} />
              <h2 id="results-heading">符合「{data.query}」的 Skill</h2>
              <p role="status" className="note">
                找到 {data.results.length} 個 Skill。
              </p>
              <MarkerLegend />
              <FacetNotes hits={data.results} />
              <ul className="search-results" aria-labelledby="results-heading">
                {data.results.map((hit) => (
                  <SearchResultRow
                    key={hit.skill_id}
                    hit={hit}
                    checked={selected.includes(hit.skill_id)}
                    atLimit={selected.length >= MAX_COMPARE}
                    onToggle={toggleSelected}
                    lifted={liftedNotes(data.results)}
                  />
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </section>
  );
}

function Catalog({
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
          <FacetNotes hits={results} />
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

const CATEGORY_CHIPS: Array<{ value: SkillCategory | undefined; label: string }> = [
  { value: undefined, label: "全部" },
  { value: "documents", label: "文件" },
  { value: "writing", label: "寫作" },
  { value: "data", label: "資料" },
];

function CategoryNav({ filters, browsing }: { filters: SearchFilters; browsing: boolean }) {
  const base: SearchFilters = { ...filters, category: undefined };
  const totals = [
    useCatalogTotal(base),
    useCatalogTotal({ ...base, category: "documents" }),
    useCatalogTotal({ ...base, category: "writing" }),
    useCatalogTotal({ ...base, category: "data" }),
  ];

  if (totals[0].data?.total === 0) return null;

  return (
    <>
      <nav className="category-nav" aria-label="分類">
        {CATEGORY_CHIPS.map(({ value, label }, index) => {
          const total = totals[index].data?.total;
          return (
            <Link
              key={label}
              className="chip"
              to="/"
              search={(prev: HomeSearch) => ({ ...prev, category: value, compare: undefined })}
              replace
              activeOptions={{ explicitUndefined: true }}
            >
              {label}（{total !== undefined ? total : totals[index].isError ? "測量失敗" : "…"}）
            </Link>
          );
        })}
      </nav>
      {!browsing && <p className="note">括號裡是目錄中各類別的數量，不是這次搜尋命中的筆數。</p>}
    </>
  );
}

const UNAVAILABLE_FILTERS: Array<{ key: string; label: string; reason: string }> = [
  {
    key: "mcp",
    label: "需要 MCP",
    reason: "靜態掃描與 SKILL.md 都沒有記錄是否需要 MCP，平台沒有這項資料可以篩。",
  },
];

function FilterBar({
  filters,
  onChange,
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
}) {
  const narrowing = Object.values(filters).some(Boolean);
  const [open, setOpen] = useState(narrowing);
  useEffect(() => {
    if (narrowing) setOpen(true);
  }, [
    narrowing,
    filters.script,
    filters.validation,
    filters.agent,
    filters.tier,
    filters.category,
  ]);

  return (
    <details
      className="filter-disclosure"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>
        篩選條件（{LIVE_FILTERS} 項可用、{UNAVAILABLE_FILTERS.length} 項目前無法篩選）
      </summary>
      <FilterControls filters={filters} onChange={onChange} />
    </details>
  );
}

const LIVE_FILTERS = 5;

function FilterControls({
  filters,
  onChange,
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
}) {
  return (
    <div className="filter-bar" role="group" aria-label="篩選條件">
      <label>
        是否包含 Script
        <select
          value={filters.script ?? ""}
          onChange={(event) =>
            onChange({ script: (event.target.value || undefined) as SearchFilters["script"] })
          }
        >
          <option value="">不限</option>
          <option value="yes">包含 Script</option>
          <option value="no">不含 Script</option>
        </select>
      </label>

      <label>
        驗證狀態
        <select
          value={filters.validation ?? ""}
          onChange={(event) =>
            onChange({
              validation: (event.target.value || undefined) as SearchFilters["validation"],
            })
          }
        >
          <option value="">不限</option>
          <option value="passed">規格驗證已通過</option>
          <option value="unverified">尚未驗證</option>
        </select>
      </label>

      <label>
        Agent 相容（實測）
        <select
          value={filters.agent ?? ""}
          onChange={(event) =>
            onChange({ agent: (event.target.value || undefined) as SearchFilters["agent"] })
          }
        >
          <option value="">不限</option>
          <option value="native">腳本可直接執行</option>
          <option value="transpiled">腳本不會執行，由模型轉譯</option>
          <option value="failed">腳本無法執行且試跑失敗</option>
          <option value="unverified">尚未試跑</option>
        </select>
      </label>

      <label>
        來源層級
        <select
          value={filters.tier ?? ""}
          aria-describedby="filter-why-tier"
          onChange={(event) =>
            onChange({ tier: (event.target.value || undefined) as SearchFilters["tier"] })
          }
        >
          <option value="">不限</option>
          <option value="curated">精選</option>
          <option value="indexed">已索引</option>
        </select>
        <span id="filter-why-tier" className="note">
          「精選」是這一版通過九項人工審查的 Skill；「已索引」是目前這一版沒有帶著人工審查結論——
          包含出了新版本、審查還沒跟上的那些，不等於從沒被審過。
        </span>
      </label>

      <label>
        類別
        <select
          value={filters.category ?? ""}
          aria-describedby="filter-why-category"
          onChange={(event) =>
            onChange({ category: (event.target.value || undefined) as SearchFilters["category"] })
          }
        >
          <option value="">不限</option>
          <option value="documents">文件</option>
          <option value="writing">寫作</option>
          <option value="data">資料</option>
        </select>
        <span id="filter-why-category" className="note">
          三個類別來自策展判定。使用者自己匯入的 Skill 目前還沒有類別，選這三個值都不會列出它們。
        </span>
      </label>

      {UNAVAILABLE_FILTERS.map(({ key, label, reason }) => (
        <label key={key} title={reason}>
          {label}
          <select disabled aria-describedby={`filter-why-${key}`}>
            <option>無法篩選</option>
          </select>
          <span id={`filter-why-${key}`} className="note">
            {reason}
          </span>
        </label>
      ))}
    </div>
  );
}

function RankingExplainer() {
  return (
    <details className="ranking-explainer">
      <summary>排序依據（為什麼是這個順序？）</summary>
      <ul>
        <li>
          <strong>排的是語意相似度。</strong>
          系統把你輸入的描述和每個 Skill 的索引文字都轉成向量，兩者越接近就排越前面。每個結果標示的
          「相似度」就是這個分數，範圍 0 到 1，越大越接近。
        </li>
        <li>
          <strong>關鍵字命中只用來多找候選，不會改變名次。</strong>
          靠關鍵字被找出來的 Skill，一樣用它自己的語意相似度排序，不會因為字面命中而往前擠。
        </li>
        <li>
          {/* 0.25 must stay in sync with catalog.MaxCosineDistance by hand; the
              search response does not carry the value. */}
          <strong>相似度低於 0.25 的一律不顯示。</strong>
          這個 0.25 是平台目前的設定值，不是介面契約的一部分，調整了這一頁不會自己跟著改。全部都低於
          門檻時會直接說「沒有夠接近的 Skill」，而不是硬給一頁不相關的結果。這個門檻是實測
          出來的：在評測語料上，12 條離題查詢全部被正確拒答，同時 48 條正常查詢一條也沒漏掉。
        </li>
        <li>
          <strong>不看 Star 數、下載數或更新時間。</strong>
          排序完全不使用人氣或新舊，只有相關度。
        </li>
        <li>
          <strong>篩選條件不影響名次。</strong>
          篩選只是把不符合條件的結果整個拿掉，剩下的順序和沒篩之前完全一樣。
        </li>
        <li>
          <strong>例外一：只能用關鍵字比對時。</strong>
          語意服務不可用的話，整頁改用關鍵字分數排序。那個分數不是相似度、沒有固定範圍，上面那個
          門檻也不生效；這時不顯示相似度，改在每個結果旁註明原因。篩選條件在這個狀態下照樣生效。
        </li>
        <li>
          <strong>例外二：還沒建立語意索引的 Skill。</strong>
          這些 Skill
          算不出相似度，只能靠關鍵字被找到，固定排在最後，門檻也沒有判斷過它們；畫面上不顯示
          相似度，改註明原因。
        </li>
      </ul>
    </details>
  );
}

function CompareBar({ selected }: { selected: string[] }) {
  return (
    <div className="compare-bar">
      {selected.length >= 2 ? (
        <Link to="/compare" search={{ ids: selected.join(",") }}>
          並排比較這 {selected.length} 個 Skill
        </Link>
      ) : (
        <p className="note">勾選 2 至 {MAX_COMPARE} 個 Skill，即可並排比較它們的靜態資料。</p>
      )}
      {selected.length >= MAX_COMPARE && (
        <p className="note" id="compare-limit">
          已經選滿 {MAX_COMPARE}{" "}
          個，並排比較最多就是這麼多；其餘的勾選框會停用，取消一個才能改選別的。
        </p>
      )}
    </div>
  );
}

function ResultFacets({ hit, lifted }: { hit: PublicSearchResult; lifted: LiftedNotes }) {
  const untested =
    hit.compatibility.capability.value === "unverified" &&
    hit.compatibility.runtime.value === "unverified";

  return (
    <dl className="result-facets">
      <dt>來源層級</dt>
      <dd>
        <LabelledBadge kind="tier" value={hit.tier} noteInRow={!lifted.tier} />
      </dd>

      <dt>類別</dt>
      <dd>
        <LabelledBadge kind="category" value={hit.category} noteInRow={!lifted.category} />
      </dd>

      <dt>相容狀態</dt>
      <dd>
        規格驗證：{hit.compatibility.spec_validation.label}
        {untested && <span className="badge badge-untested">尚未試跑</span>}
        {!lifted.compatibility && <span className="note">{hit.compatibility.note}</span>}
      </dd>

      <dt>依賴</dt>
      <dd>
        {hit.dependencies.length > 0 ? (
          hit.dependencies.join("、")
        ) : (
          <span className="note">未測量——沒有擷取到依賴資訊，不等於沒有依賴。</span>
        )}
      </dd>

      <dt>風險提示</dt>
      <dd>
        <RiskSummary risk={hit.risk} noteInRow={!lifted.risk} />
      </dd>

      <dt>最近驗證時間</dt>
      <dd>
        {hit.verified_at ? (
          <Timestamp at={hit.verified_at} />
        ) : (
          <span className="note">未測量——這個 Skill 還沒有匯入內容可以驗證。</span>
        )}
      </dd>
    </dl>
  );
}

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

function liftedNotes(hits: PublicSearchResult[]): LiftedNotes {
  return liftedNotesOf(hits, FACET_NOTES);
}

function FacetNotes({ hits }: { hits: PublicSearchResult[] }) {
  return <FacetNoteLines rows={hits} facets={FACET_NOTES} />;
}

function MarkerLegend() {
  return (
    <p className="note">
      標記說明：「AI 改寫」與「AI 產生」由模型寫成，未經人工核對——你的 Agent 讀到的是套件自己的
      description，不是這裡的改寫；「作者原文」是套件的 frontmatter
      description；「規則產生」依查詢與文件的關鍵字重疊組出；
      「來源未標示」代表伺服器沒有回報這段摘要的來源。
    </p>
  );
}

function SearchResultRow({
  hit,
  checked,
  atLimit,
  onToggle,
  rankNoteInList = true,
  lifted = {},
}: {
  hit: PublicSearchResult;
  checked: boolean;
  atLimit: boolean;
  onToggle: (skillId: string) => void;
  lifted?: LiftedNotes;
  rankNoteInList?: boolean;
}) {
  return (
    <li className="search-result">
      <Link to="/skills/$skillId" params={{ skillId: hit.skill_id }}>
        {hit.name}
      </Link>
      <label className="compare-pick">
        <input
          type="checkbox"
          checked={checked}
          disabled={!checked && atLimit}
          aria-describedby={!checked && atLimit ? "compare-limit" : undefined}
          onChange={() => onToggle(hit.skill_id)}
        />
        加入比較
      </label>
      <p>
        {hit.summary}{" "}
        {hit.summary_source === "model" && (
          <span
            className="badge badge-source-model"
            title="這段摘要由模型改寫，不是套件作者寫的；你的 Agent 讀的是套件自己的 description"
          >
            AI 改寫
          </span>
        )}
        {hit.summary_source === "package" && (
          <span className="badge badge-source-package" title="套件自己的 frontmatter description">
            作者原文
          </span>
        )}
        {hit.summary_source !== "model" && hit.summary_source !== "package" && (
          <span className="badge badge-source-unknown" title="伺服器沒有回報這段摘要的來源">
            來源未標示
          </span>
        )}
      </p>
      {hit.match_reason && (
        <p className="match-reason">
          符合原因：{hit.match_reason}
          {hit.match_reason_source === "model" && (
            <span className="badge badge-source-model" title="這段說明由模型產生，未經人工核對">
              AI 產生
            </span>
          )}
          {hit.match_reason_source === "template" && (
            <span className="badge badge-source-template" title="依查詢與文件的關鍵字重疊組出">
              規則產生
            </span>
          )}
        </p>
      )}
      <ResultFacets hit={hit} lifted={lifted} />
      {(hit.rank !== null || rankNoteInList) && (
        <p className="rank">
          {hit.rank === null
            ? (hit.rank_note ?? "未計算語意相似度。")
            : `相似度 ${hit.rank.toFixed(2)}`}
        </p>
      )}
    </li>
  );
}

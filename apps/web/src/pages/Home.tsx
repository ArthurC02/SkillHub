import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useSkillSearch } from "../api/skills";
import { LabelledBadge } from "../components/LabelledBadge";
import { MAX_COMPARE } from "./Compare";
import type { PublicSearchResult, SearchFilters } from "../api/types";

/**
 * DISC-001/002/003/004/005: public intent search. Anonymous — GET
 * /api/skills/search is mounted without RequireSession, so nothing on this page
 * needs a session.
 *
 * The empty / low-confidence / degraded states come from the server's four
 * separate flags and are shown separately, because they mean different things:
 * `no_results` = nothing was close enough, `filtered_out` = there were matches
 * but the filters removed them, `degraded` = we could not look properly,
 * `partial_index` = part of the catalog is not searchable yet.
 *
 * Query and filters live in the URL (see router.tsx), so a filtered result page
 * can be shared and survives a reload.
 */
export function Home() {
  const search = useSearch({ from: "/" });
  const navigate = useNavigate({ from: "/" });
  const [draft, setDraft] = useState(search.q ?? "");
  const [selected, setSelected] = useState<string[]>([]);

  const filters: SearchFilters = {
    script: search.script,
    validation: search.validation,
    agent: search.agent,
  };
  // `undefined` = nothing submitted yet, so no request. `""` is a blank submit,
  // which the server answers with no_results plus the suggestion copy (DISC-005);
  // duplicating that copy here is exactly what the acceptance criteria stopped
  // asking for.
  const { data, isFetching, isError } = useSkillSearch(
    search.q ?? "",
    filters,
    search.q !== undefined,
  );

  /**
   * Any change to the question — new query or new filter — makes the old
   * selection meaningless: the ids may not be on the page any more, and a
   * hidden selection would compare skills the user can no longer see.
   */
  function submitSearch(next: Partial<typeof search>) {
    setSelected([]);
    void navigate({ search: (prev) => ({ ...prev, ...next }), replace: true });
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    submitSearch({ q: draft.trim() });
  }

  function toggleSelected(skillId: string) {
    setSelected((current) =>
      current.includes(skillId)
        ? current.filter((id) => id !== skillId)
        : current.length >= MAX_COMPARE
          ? current
          : [...current, skillId],
    );
  }

  return (
    <section>
      <h1>用一句話描述你的任務</h1>
      <form onSubmit={handleSubmit}>
        <input
          type="text"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="例如：把這份 PDF 整理成摘要"
          aria-label="任務描述"
        />
        <button type="submit">搜尋</button>
      </form>

      <FilterBar
        filters={filters}
        onChange={(next) => submitSearch(next)}
        disabled={search.q === undefined}
      />

      {isFetching && <p role="status">搜尋中…</p>}
      {isError && <p role="alert">搜尋失敗，請稍後再試。</p>}

      {data && (
        <>
          {/* DISC-001: the original query is kept and echoed, not rewritten away. */}
          <p className="echoed-query">
            查詢：<q>{data.query}</q>
          </p>

          {/* Non-blocking notices: results (if any) are still shown below. */}
          {data.degraded && (
            <p className="notice" role="status">
              目前只用關鍵字比對搜尋，跨語言與語意相近的結果會找不到，召回率明顯較低。
              {data.degraded_reason && <span className="note">（{data.degraded_reason}）</span>}
            </p>
          )}
          {data.partial_index && (
            <p className="notice" role="status">
              部分 Skill 尚未建立語意索引，只能靠關鍵字命中，沒有相似度可顯示，並排在最後。
            </p>
          )}

          {/*
            DISC-003: the two empty states are different problems with different
            fixes, so they never share copy. Widening a filter and rewording a
            task are opposite advice, and giving the wrong one sends the user
            looking in a place where the answer is not.
          */}
          {data.filtered_out && (
            <div className="no-results">
              <p>有符合這個任務的 Skill，但全部被目前的篩選條件排除了。</p>
              <p className="query-suggestion">放寬或清除下方的篩選條件即可看到它們。</p>
              <button
                type="button"
                onClick={() => submitSearch({ script: undefined, validation: undefined })}
              >
                清除所有篩選
              </button>
            </div>
          )}

          {data.no_results && (
            <div className="no-results">
              <p>沒有夠接近的 Skill。</p>
              {/* DISC-005: the suggestion is the server's, not a hardcoded string. */}
              {data.query_suggestion && <p className="query-suggestion">{data.query_suggestion}</p>}
            </div>
          )}

          {data.results.length > 0 && (
            <>
              <RankingExplainer degraded={data.degraded} partialIndex={data.partial_index} />
              <CompareBar selected={selected} />
              {/*
                The count is the live region, not the list: a list of result
                cards under aria-live makes a screen reader re-read every card
                in full on each search, which is louder than saying nothing.
              */}
              <p role="status" className="note">
                找到 {data.results.length} 個 Skill。
              </p>
              <ul className="search-results" aria-label="搜尋結果">
                {data.results.map((hit) => (
                  <SearchResultRow
                    key={hit.skill_id}
                    hit={hit}
                    checked={selected.includes(hit.skill_id)}
                    atLimit={selected.length >= MAX_COMPARE}
                    onToggle={toggleSelected}
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

/**
 * DISC-003 (spec 02:DISC-002「使用者可依類別、來源層級、Agent、是否包含 Script、
 * 是否需要 MCP 與驗證狀態篩選」).
 *
 * Three of the six dimensions have per-row data in this build and are live
 * controls. The other three are rendered as disabled controls carrying the
 * reason, rather than being hidden or — far worse — offered as controls that
 * accept a value and narrow nothing. The server rejects those three with 400 for
 * the same reason, so a hand-edited URL cannot get an unfiltered page that looks
 * filtered.
 *
 * The wording of each reason is the honest one, not a "coming soon": MCP has no
 * source of truth anywhere in the pipeline, and 類別/來源層級 are curation data
 * the platform never stored.
 *
 * Agent 相容 became live with the M2 baseline measurements (0022): 45 skills,
 * one sandbox Run each. Only its runtime axis is a filter — every measured skill
 * came back `activated`, so a capability filter would separate nothing, which is
 * exactly why 來源層級 is still disabled.
 */
const UNAVAILABLE_FILTERS: Array<{ key: string; label: string; reason: string }> = [
  {
    key: "category",
    label: "類別",
    reason: "類別目前只存在於策展清單，沒有存進平台，匯入流程也不收這個欄位，因此無法篩選。",
  },
  {
    key: "tier",
    label: "來源層級",
    reason: "目前目錄內每一個 Skill 都是「已索引」層級，人工精選審查尚未開始，篩了也分不出東西。",
  },
  {
    key: "mcp",
    label: "需要 MCP",
    reason: "靜態掃描與 SKILL.md 都沒有記錄是否需要 MCP，平台沒有這項資料可以篩。",
  },
];

function FilterBar({
  filters,
  onChange,
  disabled,
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
  disabled: boolean;
}) {
  return (
    <div className="filter-bar" role="group" aria-label="篩選條件">
      <label>
        是否包含 Script
        <select
          value={filters.script ?? ""}
          disabled={disabled}
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
          disabled={disabled}
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

      {/*
        The runtime axis of DISC-002's Agent dimension. The wording says what the
        value means rather than repeating the value: 「模型轉譯」 is the one a
        reader cannot guess, and it is the one that decides whether the Skill's
        own script is what runs.
      */}
      <label>
        Agent 相容（實測）
        <select
          value={filters.agent ?? ""}
          disabled={disabled}
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

      {UNAVAILABLE_FILTERS.map(({ key, label, reason }) => (
        <label key={key} className="filter-unavailable" title={reason}>
          {label}
          <select disabled aria-describedby={`filter-why-${key}`}>
            <option>無法篩選</option>
          </select>
          <span id={`filter-why-${key}`} className="note">
            {reason}
          </span>
        </label>
      ))}

      {/*
        DISC-003 honesty note, sitting with the controls rather than in a help
        page: a filter bar with four dead controls has to say why on the spot,
        or it reads as a broken UI instead of an absent capability.
      */}
      <p className="note">
        篩選條件只會用平台真的有的資料。上面標為「無法篩選」的三項，是因為平台目前沒有這些資料，
        不是因為所有 Skill 都不符合。
      </p>
    </div>
  );
}

/**
 * DISC-004 (spec 02:DISC-002 「排序依據需可被簡要說明」): what actually decides
 * the order, in plain language.
 *
 * Every claim below is the measured behaviour of the pipeline this page calls,
 * not an ideal: ranking is vector distance alone and the lexical leg only
 * widens the candidate set (db/queries/search.sql `PublicHybridSearchSkills`,
 * after golden-query-set.md §10.7 measured equal-weight RRF costing 15 of 48
 * queries their Top-1). The two exceptions are the two states the response
 * already flags, and they are listed whether or not they apply right now —
 * the rule is what is being explained — with the live one marked.
 */
function RankingExplainer({
  degraded,
  partialIndex,
}: {
  degraded: boolean;
  partialIndex: boolean;
}) {
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
          {/*
            ponytail: 0.25 is transcribed from catalog.MaxCosineDistance (0.75
            cosine distance). The contract does not expose it, and one hardcoded
            number beats a new endpoint for it — move it onto the search
            response the next time the value changes.
          */}
          <strong>相似度低於 0.25 的一律不顯示。</strong>
          全部都低於門檻時會直接說「沒有夠接近的 Skill」，而不是硬給一頁不相關的結果。這個門檻是實測
          出來的：在評測語料上，12 條離題查詢全部被正確拒答，同時 48 條正常查詢一條也沒漏掉。
        </li>
        <li>
          <strong>不看 Star 數、下載數或更新時間。</strong>
          排序完全不使用人氣或新舊，只有相關度。
        </li>
        <li>
          {/*
            DISC-003: filtering is not ranking. Saying so here is what stops a
            user reading a short filtered page as "the search got worse".
          */}
          <strong>篩選條件不影響名次。</strong>
          篩選只是把不符合條件的結果整個拿掉，剩下的順序和沒篩之前完全一樣。
        </li>
        <li>
          <strong>例外一：只能用關鍵字比對時。</strong>
          語意服務不可用的話，整頁改用關鍵字分數排序。那個分數不是相似度、沒有固定範圍，上面的 0.25
          門檻也不生效；這時不顯示相似度，改在每個結果旁註明原因。篩選條件在這個狀態下照樣生效。
          {degraded && <span className="badge">目前這次搜尋就是這個狀態</span>}
        </li>
        <li>
          <strong>例外二：還沒建立語意索引的 Skill。</strong>
          這些 Skill
          算不出相似度，只能靠關鍵字被找到，固定排在最後，門檻也沒有判斷過它們；畫面上不顯示
          相似度，改註明原因。
          {partialIndex && <span className="badge">這頁就有這種結果</span>}
        </li>
      </ul>
    </details>
  );
}

/** DISC-009 entry point: 2–3 selected candidates go to the comparison page. */
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
    </div>
  );
}

/**
 * 02:DISC-002 requires every result row to show name, plain summary, source
 * tier, agent compatibility, dependencies, a risk hint and the last
 * verification time. The API carries all seven; this renders the five that are
 * not the name and summary.
 *
 * Nothing here is inferred. An unscanned row says its scan status is unknown
 * rather than showing no flags, an empty dependency list says "not extracted"
 * rather than "none", and the two sandbox compatibility axes are marked
 * 尚未試跑 rather than left out — an absent row reads as "fine" (NFR-001).
 */
const RISK_FLAGS: Array<{ key: keyof PublicSearchResult["risk"]; label: string }> = [
  { key: "has_scripts", label: "含 Script 檔案" },
  { key: "has_embedded_script", label: "SKILL.md 內含程式碼" },
  { key: "has_external_urls", label: "含外部網址" },
  { key: "has_possible_secrets", label: "疑似含 Secret" },
  { key: "has_binaries", label: "含二進位檔案" },
  { key: "has_dependency_manifest", label: "含依賴宣告檔" },
];

function ResultFacets({ hit }: { hit: PublicSearchResult }) {
  const flags = RISK_FLAGS.filter(({ key }) => hit.risk[key] === true);
  const untested =
    hit.compatibility.capability === "unverified" && hit.compatibility.runtime === "unverified";

  return (
    <dl className="result-facets">
      <dt>來源層級</dt>
      <dd>
        <LabelledBadge kind="tier" value={hit.tier} />
      </dd>

      <dt>相容狀態</dt>
      <dd title={hit.compatibility.note}>
        規格驗證：{hit.compatibility.spec_validation === "passed" ? "通過" : "未驗證"}
        {/* DISC-002: 沒有驗證證據的 Skill 必須明確標記「尚未試跑」. */}
        {untested && <span className="badge badge-untested">尚未試跑</span>}
      </dd>

      <dt>依賴</dt>
      <dd>
        {hit.dependencies.length > 0 ? (
          hit.dependencies.join("、")
        ) : (
          <span className="note">未擷取到依賴資訊（不等於沒有依賴）</span>
        )}
      </dd>

      <dt>風險提示</dt>
      <dd title={hit.risk.note}>
        {hit.risk.scan_status === "unavailable" ? (
          <span className="note">尚無掃描紀錄，狀態未知——不代表已通過檢查。</span>
        ) : (
          <>
            {hit.risk.warnings > 0 && (
              <span className="badge badge-risk">警告 {hit.risk.warnings}</span>
            )}
            {flags.length > 0
              ? flags.map(({ key, label }) => (
                  <span key={key} className="badge badge-risk-flag">
                    {label}
                  </span>
                ))
              : hit.risk.warnings === 0 && (
                  <span className="note">靜態掃描未發現警告；這不等於安全。</span>
                )}
          </>
        )}
      </dd>

      <dt>最近驗證時間</dt>
      <dd>
        {hit.verified_at ? (
          hit.verified_at.slice(0, 10)
        ) : (
          <span className="note">尚未匯入內容</span>
        )}
      </dd>
    </dl>
  );
}

function SearchResultRow({
  hit,
  checked,
  atLimit,
  onToggle,
}: {
  hit: PublicSearchResult;
  checked: boolean;
  atLimit: boolean;
  onToggle: (skillId: string) => void;
}) {
  return (
    <li className="search-result">
      <label className="compare-pick">
        <input
          type="checkbox"
          checked={checked}
          disabled={!checked && atLimit}
          onChange={() => onToggle(hit.skill_id)}
        />
        加入比較
      </label>
      <Link to="/skills/$skillId" params={{ skillId: hit.skill_id }}>
        {hit.name}
      </Link>
      <p>{hit.summary}</p>
      {hit.match_reason && (
        <p className="match-reason">
          <span className="match-reason-label">符合原因：</span>
          {hit.match_reason}
          {/*
            ADR-013: model-generated copy has to be visibly marked as such, so
            a reader can tell an LLM's explanation from a mechanical one derived
            from keyword overlap.
          */}
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
      <ResultFacets hit={hit} />
      {/*
        DISC-004: every candidate shows the score the order was built from.
        `rank` is null exactly when there is no similarity to show — the whole
        page came from the lexical leg, or this row has no embedding yet — and
        the server's `rank_note` says which. The unbounded lexical score is not
        substituted for it: a live FTS-only answer measured 1.4, and printing
        that under a 「相似度」 label would state a number it does not mean.
      */}
      <p className="rank">
        {hit.rank === null
          ? (hit.rank_note ?? "未計算語意相似度。")
          : `相似度 ${hit.rank.toFixed(2)}`}
      </p>
    </li>
  );
}

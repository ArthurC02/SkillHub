import { Link } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useSkillSearch } from "../api/skills";
import { MAX_COMPARE } from "./Compare";
import type { PublicSearchResult } from "../api/types";

/**
 * DISC-001/002/004/005: public intent search. Anonymous — GET
 * /api/skills/search is mounted without RequireSession, so nothing on this page
 * needs a session.
 *
 * The empty / low-confidence / degraded states come from the server's three
 * separate flags and are shown separately, because they mean different things:
 * `no_results` = nothing was close enough, `degraded` = we could not look
 * properly, `partial_index` = part of the catalog is not searchable yet.
 */
export function Home() {
  const [query, setQuery] = useState("");
  // null = not searched yet. A blank submit is still sent: the server answers
  // it with no_results plus the suggestion copy (DISC-005), and duplicating
  // that copy here is exactly what the acceptance criteria stopped asking for.
  const [submitted, setSubmitted] = useState<string | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const { data, isFetching, isError } = useSkillSearch(submitted ?? "", submitted !== null);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitted(query.trim());
    // A new result set makes the old selection meaningless: the ids may not be
    // on the page any more, and a hidden selection would compare skills the
    // user can no longer see.
    setSelected([]);
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
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="例如：把這份 PDF 整理成摘要"
          aria-label="任務描述"
        />
        <button type="submit">搜尋</button>
      </form>

      {isFetching && <p>搜尋中…</p>}
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
              <ul className="search-results">
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
          <strong>例外一：只能用關鍵字比對時。</strong>
          語意服務不可用的話，整頁改用關鍵字分數排序。那個分數不是相似度、沒有固定範圍，上面的 0.25
          門檻也不生效；這時不顯示相似度，改在每個結果旁註明原因。
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

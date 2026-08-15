import { Link } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useSkillSearch } from "../api/skills";
import type { PublicSearchResult } from "../api/types";

/**
 * DISC-001/002/005: public intent search. Anonymous — GET /api/skills/search is
 * mounted without RequireSession, so nothing on this page needs a session.
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
  const { data, isFetching, isError } = useSkillSearch(submitted ?? "", submitted !== null);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitted(query.trim());
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
              部分 Skill 尚未建立語意索引，只能靠關鍵字命中，排序分數顯示為 0 並排在最後。
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
            <ul className="search-results">
              {data.results.map((hit) => (
                <SearchResultRow key={hit.skill_id} hit={hit} degraded={data.degraded} />
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  );
}

function SearchResultRow({ hit, degraded }: { hit: PublicSearchResult; degraded: boolean }) {
  return (
    <li className="search-result">
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
        `rank` is only a 0..1 cosine similarity on the hybrid path. On the
        degraded path the server returns the lexical score instead, which is
        unbounded — a live FTS-only answer came back with 1.4 — so labelling it
        「相似度」 there would state a number the value does not mean.
      */}
      <p className="rank">
        {degraded
          ? "關鍵字比對命中（未計算語意相似度）"
          : hit.rank > 0
            ? `相似度 ${hit.rank.toFixed(2)}`
            : "未評分（尚未建立語意索引）"}
      </p>
    </li>
  );
}

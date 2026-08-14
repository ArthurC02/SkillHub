import { Link } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useSkillSearch } from "../api/skills";

export function Home() {
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const { data, isFetching, isError } = useSkillSearch(submittedQuery);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmittedQuery(query.trim());
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
      {data && submittedQuery && data.results.length === 0 && (
        <p>沒有符合的 Skill，試試補充任務、輸入或預期輸出。</p>
      )}
      {data && data.results.length > 0 && (
        <ul>
          {data.results.map((hit) => (
            <li key={hit.skill_id}>
              <Link to="/skills/$skillId" params={{ skillId: hit.skill_id }}>
                {hit.name}
              </Link>
              <p>{hit.summary}</p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

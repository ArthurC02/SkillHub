import { Link } from "@tanstack/react-router";
import type { FormEvent } from "react";

export function SearchHeroForm({
  draft,
  queryError,
  onDraftChange,
  onSubmit,
}: {
  draft: string;
  queryError: string;
  onDraftChange: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <div className="hero">
      <h1>用一句話描述你的任務</h1>
      <form onSubmit={onSubmit}>
        <input
          type="text"
          value={draft}
          onChange={(event) => onDraftChange(event.target.value)}
          placeholder="在目錄裡找一個 Skill，例如：把這份 PDF 整理成摘要"
          aria-label="任務描述"
          aria-invalid={queryError ? true : undefined}
          aria-describedby={queryError ? "search-query-error" : undefined}
        />
        {queryError && (
          <p role="alert" id="search-query-error">
            {queryError}
          </p>
        )}
        <button type="submit" className="action">
          搜尋
        </button>
        <Link className="hero-create" to="/library" hash="create">
          自己做一個 Skill
        </Link>
      </form>
    </div>
  );
}

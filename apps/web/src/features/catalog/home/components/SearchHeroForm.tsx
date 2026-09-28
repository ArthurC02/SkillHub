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
  );
}

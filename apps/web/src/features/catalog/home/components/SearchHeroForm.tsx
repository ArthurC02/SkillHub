import type { FormEvent } from "react";

export function SearchHeroForm({
  compact = false,
  draft,
  queryError,
  onDraftChange,
  onSubmit,
}: {
  compact?: boolean;
  draft: string;
  queryError: string;
  onDraftChange: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <div className="hero">
      <p className="hero-kicker">小工具目錄</p>
      <h1>{compact ? "搜尋 Agent 小工具" : "探索能直接採用的 Agent 小工具"}</h1>
      <p className="hero-lede">先看平台收錄的能力，再用任務、輸入或輸出縮小範圍。</p>
      <form aria-label="搜尋小工具目錄" onSubmit={onSubmit}>
        <input
          type="text"
          value={draft}
          onChange={(event) => onDraftChange(event.target.value)}
          placeholder="例如：把 PDF 整理成摘要、清理 CSV 欄位"
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
      </form>
    </div>
  );
}

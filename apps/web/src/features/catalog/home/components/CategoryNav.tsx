import { Link } from "@tanstack/react-router";
import { useLayoutEffect } from "react";
import { useCatalogTotal } from "../../../skill";
import type { HomeSearch } from "../../../../app/router";
import type { SearchFilters, SkillCategory } from "../../../../core/api/types";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../../shared/ui/LoginRequired.model";
import { Loading } from "../../../../shared/ui/Loading";

const CATEGORY_CHIPS: Array<{ value: SkillCategory | undefined; label: string }> = [
  { value: undefined, label: "全部" },
  { value: "documents", label: "文件" },
  { value: "writing", label: "寫作" },
  { value: "data", label: "資料" },
];

export function CategoryNav({
  filters,
  browsing,
  correction,
  onCountsChange,
}: {
  filters: SearchFilters;
  browsing: boolean;
  correction?: string;
  onCountsChange: () => void;
}) {
  const base: SearchFilters = { ...filters, category: undefined };
  const totals = [
    useCatalogTotal(base),
    useCatalogTotal({ ...base, category: "documents" }),
    useCatalogTotal({ ...base, category: "writing" }),
    useCatalogTotal({ ...base, category: "data" }),
  ];

  const counts = totals.map((query) =>
    query.isSuccess && query.fetchStatus === "idle" ? query.data : undefined,
  );
  const failed =
    totals.find((query) => unauthenticated(query.error)) ?? totals.find((query) => query.isError);
  const busy = totals.some((query) => query.fetchStatus !== "idle");
  const paused = totals.some((query) => query.fetchStatus === "paused");
  const layout = JSON.stringify(
    totals.map(({ status, fetchStatus, data }) => [status, fetchStatus, data]),
  );
  useLayoutEffect(() => onCountsChange(), [onCountsChange, layout]);

  if (counts.every((count) => count === 0)) return null;

  return (
    <>
      <nav className="category-nav" aria-label="分類" aria-busy={busy}>
        {CATEGORY_CHIPS.map(({ value, label }, index) => {
          const total = counts[index];
          return (
            <Link
              key={label}
              className="chip"
              to="/"
              search={(prev: HomeSearch) => ({
                ...prev,
                ...filters,
                correction,
                category: value,
              })}
              replace
              activeOptions={{ explicitUndefined: true }}
            >
              {label}（{total !== undefined ? total : totals[index].isError ? "測量失敗" : "…"}）
            </Link>
          );
        })}
      </nav>
      <ReadFailure
        error={failed?.error}
        what="分類數量"
        retry={() => void Promise.all(totals.map((query) => query.refetch()))}
        retrying={busy}
        focusTarget="catalog-search-heading"
      />
      {paused && <Loading what="分類數量" paused />}
      {!browsing && <p className="note">括號裡是目錄中各類別的數量，不是這次搜尋命中的筆數。</p>}
    </>
  );
}

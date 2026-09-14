import { Link } from "@tanstack/react-router";
import { useCatalogTotal } from "../../../skill";
import type { HomeSearch } from "../../../../app/router";
import type { SearchFilters, SkillCategory } from "../../../../core/api/types";

const CATEGORY_CHIPS: Array<{ value: SkillCategory | undefined; label: string }> = [
  { value: undefined, label: "全部" },
  { value: "documents", label: "文件" },
  { value: "writing", label: "寫作" },
  { value: "data", label: "資料" },
];

export function CategoryNav({ filters, browsing }: { filters: SearchFilters; browsing: boolean }) {
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

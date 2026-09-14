import type { SearchFilters } from "../../../../core/api/types";

export const UNAVAILABLE_FILTERS: Array<{ key: string; label: string; reason: string }> = [
  {
    key: "mcp",
    label: "需要 MCP",
    reason: "靜態掃描與 SKILL.md 都沒有記錄是否需要 MCP，平台沒有這項資料可以篩。",
  },
];

export function FilterControls({
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

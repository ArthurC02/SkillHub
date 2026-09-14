import type { CreationReference } from "../../creation.service";

function declaredReferenceField(value?: string) {
  return value?.trim() ? value : "未宣告";
}

const TIER_LABEL: Record<string, string> = { curated: "精選", indexed: "已索引" };

function referenceTierLabel(tier?: string): string {
  return TIER_LABEL[tier ?? ""] ?? "不在目錄";
}

function referenceScanLabel(scanStatus?: string, warnings?: number): string {
  if (scanStatus === "scanned")
    return (warnings ?? 0) > 0 ? `已掃描，${warnings} 個警告` : "已掃描，無警告";
  if (scanStatus === "unavailable") return "沒有掃描紀錄";
  return "未知";
}

export function ReferenceList({
  items,
  adoptable,
  showStatus,
  locked,
  onAdopt,
}: {
  items: CreationReference[];
  adoptable: boolean;
  showStatus: boolean;
  locked: boolean;
  onAdopt: (skillID: string) => void;
}) {
  return (
    <ul className="ref-list">
      {items.map((r) => (
        <li key={r.skill_id}>
          <div className="ref-head">
            <strong>{r.name}</strong>
            {showStatus && (
              <span className="card-tag" data-tone={r.confirmed ? "done" : undefined}>
                {!r.available ? "目前不可用" : r.confirmed ? "已確認" : "尚未確認"}
              </span>
            )}
          </div>
          <p>{declaredReferenceField(r.description)}</p>
          <ul className="ref-facts">
            <li>相容：{declaredReferenceField(r.compatibility)}</li>
            <li>工具：{declaredReferenceField(r.allowed_tools)}</li>
            <li>{referenceTierLabel(r.tier)}</li>
            <li>{referenceScanLabel(r.scan_status, r.warnings)}</li>
          </ul>
          <details>
            <summary>固定版本</summary>
            {r.version_id}
          </details>
          {adoptable && (
            <div className="ref-adopt">
              <button disabled={locked || !r.available} onClick={() => onAdopt(r.skill_id)}>
                直接採用
              </button>
              {r.scan_status !== "scanned" && (
                <span className="note">沒有掃描紀錄，不建議直接採用</span>
              )}
            </div>
          )}
        </li>
      ))}
    </ul>
  );
}

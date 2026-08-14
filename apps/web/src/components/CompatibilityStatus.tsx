import type { CompatibilityEntry, CompatibilityResult } from "../api/types";

const LABELS: Record<CompatibilityResult, string> = {
  unverified: "尚未驗證",
  passed: "相容",
  failed: "不相容",
};

export function CompatibilityStatus({ entries }: { entries: CompatibilityEntry[] }) {
  if (entries.length === 0) {
    return <span className="badge badge-compat-unverified">{LABELS.unverified}</span>;
  }

  return (
    <ul className="compat-list">
      {entries.map((entry) => (
        <li key={`${entry.agent}-${entry.runtime ?? ""}`} className={`badge badge-compat-${entry.status}`}>
          {entry.agent}
          {entry.runtime ? `（${entry.runtime}）` : ""}：{LABELS[entry.status]}
        </li>
      ))}
    </ul>
  );
}

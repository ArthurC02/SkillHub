import type { FindingSeverity, SeverityCounts, SkillRisk } from "../../../../core/api/types";

const SEVERITY_LABEL: Record<FindingSeverity, string> = {
  error: "錯誤",
  warning: "警告",
  info: "提示",
};

function highestSeverity(counts: SeverityCounts): FindingSeverity | null {
  if (counts.errors > 0) return "error";
  if (counts.warnings > 0) return "warning";
  if (counts.infos > 0) return "info";
  return null;
}

export function RiskVerdict({ risk }: { risk: SkillRisk }) {
  if (risk.scan_status === "unavailable") {
    return <p className="badge badge-risk">風險掃描結果未知：無法讀取已保存的套件內容。</p>;
  }
  const total = risk.counts.errors + risk.counts.warnings + risk.counts.infos;
  const highest = highestSeverity(risk.counts);
  return (
    <p className="risk-counts">
      {highest
        ? `有 ${total} 項風險，最高為${SEVERITY_LABEL[highest]}。`
        : "靜態掃描未發現錯誤、警告或提示；這不等於安全。"}
    </p>
  );
}

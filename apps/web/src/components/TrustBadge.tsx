import type { SourceTrustLevel } from "../api/types";

const LABELS: Record<SourceTrustLevel, string> = {
  unknown: "來源未知",
  traceable: "來源可追溯",
  confirmed: "來源已人工確認",
};

export function TrustBadge({ level }: { level: SourceTrustLevel }) {
  return <span className={`badge badge-trust-${level}`}>{LABELS[level]}</span>;
}

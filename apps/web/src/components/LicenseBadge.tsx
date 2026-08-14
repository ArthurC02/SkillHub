import type { LicenseState } from "../api/types";

// DISC-003 AC: unknown license must show "授權未知", never imply the skill
// is freely modifiable or redistributable.
const LABELS: Record<LicenseState, string> = {
  unknown: "授權未知",
  declared: "授權已宣告",
  confirmed: "授權已人工確認",
};

export function LicenseBadge({ status, name }: { status: LicenseState; name?: string }) {
  return (
    <span className={`badge badge-license-${status}`}>
      {LABELS[status]}
      {status !== "unknown" && name ? `：${name}` : ""}
    </span>
  );
}

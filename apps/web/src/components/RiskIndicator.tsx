import type { SkillRisk } from "../api/types";

const RISK_LABELS: Array<{ key: keyof SkillRisk; label: string }> = [
  { key: "has_scripts", label: "含可執行 Script" },
  { key: "has_external_urls", label: "含外部網址" },
  { key: "has_possible_secrets", label: "疑似含 Secret" },
];

// DISC-008 AC: no single "safe" badge — each risk factor is its own
// indicator, and the absence of flags is worded as "no flags detected",
// not "safe".
export function RiskIndicator({ risk }: { risk: SkillRisk }) {
  const active = RISK_LABELS.filter(({ key }) => risk[key]);

  if (active.length === 0) {
    return <span className="badge badge-risk-none">未偵測到風險提示</span>;
  }

  return (
    <ul className="risk-list">
      {active.map(({ key, label }) => (
        <li key={key} className="badge badge-risk">
          {label}
        </li>
      ))}
    </ul>
  );
}

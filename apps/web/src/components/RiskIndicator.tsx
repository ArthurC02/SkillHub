import type { SkillRisk } from "../api/types";

/**
 * DISC-008 risk disclosure. Deliberately no single "safe" badge (NFR-001):
 * errors and warnings are shown verbatim and up front, info-level disclosures
 * are aggregated behind a disclosure control because one seed package produced
 * 321 URL findings and a list nobody reads hides the ones that matter.
 *
 * `scan_status: "unavailable"` is rendered as unknown, never as a clean scan
 * (DISC-004 不得自行推定為通過).
 */
const FLAG_LABELS: Array<{ key: keyof SkillRisk; label: string }> = [
  { key: "has_scripts", label: "含可執行 Script 檔案" },
  // SKILL-003: separate from has_scripts because no file list can show it.
  { key: "has_embedded_script", label: "SKILL.md 內含可執行程式碼" },
  { key: "has_external_urls", label: "含外部網址" },
  { key: "has_possible_secrets", label: "疑似含 Secret" },
  { key: "has_binaries", label: "含二進位檔案" },
];

export function RiskIndicator({ risk }: { risk: SkillRisk }) {
  if (risk.scan_status === "unavailable") {
    return (
      <div>
        <p className="badge badge-risk">風險掃描結果未知：無法讀取已保存的套件內容。</p>
        <p className="note">{risk.note}</p>
      </div>
    );
  }

  const flags = FLAG_LABELS.filter(({ key }) => risk[key] === true);
  const infoCodes = Object.entries(risk.info_counts).sort(([a], [b]) => a.localeCompare(b));

  return (
    <div>
      <p className="risk-counts">
        錯誤 {risk.counts.errors}／警告 {risk.counts.warnings}／提示 {risk.counts.infos}
      </p>

      {risk.highlights.length > 0 && (
        <ul className="risk-list">
          {risk.highlights.map((finding, i) => (
            <li key={`${finding.code}-${finding.path ?? ""}-${i}`} className="badge badge-risk">
              <strong>{finding.severity === "error" ? "錯誤" : "警告"}</strong>
              <span className="risk-code">{finding.code}</span>
              {finding.path && <code>{finding.path}</code>}
              <span>{finding.message}</span>
            </li>
          ))}
        </ul>
      )}

      {flags.length > 0 && (
        <ul className="risk-list">
          {flags.map(({ key, label }) => (
            <li key={key} className="badge badge-risk-flag">
              {label}
            </li>
          ))}
        </ul>
      )}

      {/* Untinted on purpose (§4.4): the two tints mean 不通過 and 未知, and a
          clean scan is neither — the sentence, not a colour, is what stops it
          reading as a 「安全」 badge (NFR-001). `badge-risk-none` carried no rule
          and no meaning, so it is gone rather than left as a hook. */}
      {risk.highlights.length === 0 && flags.length === 0 && (
        <p className="badge">靜態掃描未發現錯誤或警告；這不等於安全。</p>
      )}

      {/* `risk-infos` has no CSS rule, but it is not dead markup either:
          disc.test.tsx selects `details.risk-infos` to assert the 321 info
          findings stay collapsed. It is a test hook, so it stays. */}
      {infoCodes.length > 0 && (
        <details className="risk-infos">
          <summary>提示層級揭露（{risk.counts.infos} 項，依類型彙總）</summary>
          <ul className="risk-list">
            {infoCodes.map(([code, count]) => (
              <li key={code}>
                <span className="risk-code">{code}</span> × {count}
              </li>
            ))}
          </ul>
        </details>
      )}

      <p className="note">{risk.note}</p>
    </div>
  );
}

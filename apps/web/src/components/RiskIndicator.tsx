import type { SearchResultRisk, SkillRisk } from "../api/types";

/**
 * DISC-008 risk disclosure. Deliberately no single "safe" badge (NFR-001):
 * errors and warnings are shown verbatim and up front, info-level disclosures
 * are aggregated behind a disclosure control because one seed package produced
 * 321 URL findings and a list nobody reads hides the ones that matter.
 *
 * `scan_status: "unavailable"` is rendered as unknown, never as a clean scan
 * (DISC-004 不得自行推定為通過).
 */
/**
 * One list, because there were two and they disagreed.
 *
 * The search row kept its own copy with weaker wording — 「含 Script 檔案」 against
 * 「含可執行 Script 檔案」, 「SKILL.md 內含程式碼」 against 「SKILL.md 內含可執行程式碼」 —
 * so the same boolean disclosed less on the screen a reader meets first. 可執行 is
 * the word doing the work in both; dropping it is not a shorter label, it is a
 * smaller claim. 設計 §4.4 exists for exactly this shape.
 *
 * Keyed by name rather than by `keyof`, because the two payloads carry different
 * sets: `SearchResultRisk` has `has_dependency_manifest` and `SkillRisk` does not,
 * which means **the detail view discloses one thing less than the list row**. That
 * asymmetry is in the contract (04 丙-29), not here; this list is written so that
 * closing it needs no change on this side.
 */
export const RISK_DISCLOSURES: Array<{ key: string; label: string }> = [
  { key: "has_scripts", label: "含可執行 Script 檔案" },
  // SKILL-003: separate from has_scripts because no file list can show it.
  { key: "has_embedded_script", label: "SKILL.md 內含可執行程式碼" },
  { key: "has_external_urls", label: "含外部網址" },
  { key: "has_possible_secrets", label: "疑似含 Secret" },
  { key: "has_binaries", label: "含二進位檔案" },
  { key: "has_dependency_manifest", label: "含依賴宣告檔" },
];

/** The disclosures a risk payload actually asserts, in the order above. */
export function disclosures(risk: object): Array<{ key: string; label: string }> {
  const flags = risk as Record<string, unknown>;
  return RISK_DISCLOSURES.filter((f) => flags[f.key] === true);
}

/**
 * The compact form: the projected block a row carries, not a fresh scan. Used by
 * the public search row and by the owner's own skill list, which is the whole
 * reason it is a component — the two are the same fact about the same skill and
 * 02:NFR-007 第 3 條 does not allow them to be worded independently.
 *
 * The `unavailable` branch renders only the server's note. It used to sit beside
 * a hardcoded 「尚無掃描紀錄，狀態未知——不代表已通過檢查。」, which is the server's
 * own sentence with different punctuation, so that row said the same thing twice
 * in two spellings.
 */
export function RiskSummary({ risk }: { risk: SearchResultRisk }) {
  const flags = disclosures(risk);
  return (
    <>
      {risk.scan_status === "scanned" && (
        <>
          {risk.warnings > 0 && <span className="badge badge-risk">警告 {risk.warnings}</span>}
          {flags.length > 0
            ? flags.map(({ key, label }) => (
                <span key={key} className="badge badge-risk-flag">
                  {label}
                </span>
              ))
            : risk.warnings === 0 && (
                <span className="note">靜態掃描未發現警告；這不等於安全。</span>
              )}
        </>
      )}
      <span className="note">{risk.note}</span>
    </>
  );
}

export function RiskIndicator({ risk }: { risk: SkillRisk }) {
  if (risk.scan_status === "unavailable") {
    return (
      <div>
        <p className="badge badge-risk">風險掃描結果未知：無法讀取已保存的套件內容。</p>
        <p className="note">{risk.note}</p>
      </div>
    );
  }

  const flags = disclosures(risk);
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

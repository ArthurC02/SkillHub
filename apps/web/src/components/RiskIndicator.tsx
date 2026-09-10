import type { Disclosure, SearchResultRisk, SkillRisk } from "../api/types";
import { StateIcon } from "./StateIcon";

function RiskLevel({ risk }: { risk: { level?: string } }) {
  if (risk.level !== "unknown") return null;
  return (
    <>
      <span className="badge badge-unverified">
        <StateIcon state="unknown" />
        未掃描
      </span>
      <span className="note">
        這個版本沒有靜態掃描結果可讀，所以這裡沒有任何發現可以顯示——那不是「掃過了、沒發現」。
      </span>
    </>
  );
}

export function RiskSummary({
  risk,
  noteInRow = true,
}: {
  risk: SearchResultRisk;
  noteInRow?: boolean;
}) {
  const flags = risk.disclosures;
  return (
    <>
      <RiskLevel risk={risk} />
      {risk.scan_status === "scanned" && (
        <>
          {risk.warnings > 0 && (
            <span className="badge badge-risk">
              <StateIcon state="fail" />
              警告 {risk.warnings}
            </span>
          )}
          {flags.length > 0
            ? flags.map((d: Disclosure) => (
                <span key={d.code}>
                  <span className="badge badge-risk-flag">
                    <StateIcon state="fail" />
                    {d.label}
                  </span>
                  {d.note && <span className="note">{d.note}</span>}
                </span>
              ))
            : risk.warnings === 0 && (
                <span className="note">靜態掃描未發現警告；這不等於安全。</span>
              )}
        </>
      )}
      {noteInRow && <span className="note">{risk.note}</span>}
    </>
  );
}

export function RiskIndicator({ risk }: { risk: SkillRisk }) {
  if (risk.scan_status === "unavailable") {
    return (
      <div>
        <p className="badge badge-risk">
          <StateIcon state="unknown" />
          風險掃描結果未知：無法讀取已保存的套件內容。
        </p>
        <p className="note">{risk.note}</p>
      </div>
    );
  }

  const flags = risk.disclosures;
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
              <StateIcon state="fail" />
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
          {flags.map((d: Disclosure) => (
            <li key={d.code} className="badge badge-risk-flag">
              <StateIcon state="fail" />
              {d.label}
              {d.note && <span className="note">{d.note}</span>}
            </li>
          ))}
        </ul>
      )}

      {risk.highlights.length === 0 && flags.length === 0 && (
        <p className="badge">
          <StateIcon state="pass" />
          靜態掃描未發現錯誤或警告；這不等於安全。
        </p>
      )}

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

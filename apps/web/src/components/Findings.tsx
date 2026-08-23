import type { CategorizedFindings, ImportFinding } from "../api/import";

/**
 * SKILL-002/INGEST-008's findings, verbatim and grouped by severity.
 *
 * Lifted out of ImportSkill on 2026-08-23 because generation needs the SAME
 * renderer, not a similar one: 02:GEN-003 says a failed generation hands back
 * `skillpkg.Validate`'s findings 逐字, exactly as a failed import does. Two
 * copies is how one of the two screens quietly starts rewriting them into
 * reassurance.
 */
export function Findings({ findings }: { findings: CategorizedFindings }) {
  const groups = [
    ["阻擋錯誤", findings.errors],
    ["警告", findings.warnings],
    ["資訊", findings.infos],
  ] as const;

  // §2.1. Nothing found is a result, and it was the one this page rendered as
  // an empty div. What the scan does not cover is stated with it, because a
  // clean report is exactly when a reader is most likely to read 沒有發現 as
  // 有人看過並認可.
  if (groups.every(([, items]) => items.length === 0)) {
    return (
      <p className="note">
        靜態檢查跑完了，沒有任何發現——這是「掃過了，沒掃到」，不是「沒掃」。它讀套件內容、不執行其中的
        Script，<strong>既不是人工審查，也不是簽章驗證</strong>
        ：MVP 的套件不帶數位簽章，平台也不驗簽（ADR-027 決策 3
        是明文的「不做」），所以簽章這一項不是還沒驗，是這裡永遠不會有人替你驗。
      </p>
    );
  }

  return (
    <div>
      {groups.map(([label, items]) =>
        items.length ? (
          <section key={label}>
            <h3>
              {label}（{items.length}）
            </h3>
            {/* §4.3: a finding is the definition of 一則可以被單獨採信或拒絕的
                東西 — a code, a path and a claim the reader weighs before
                trusting the package — and it was a bare UA `<ul>` with discs.
                `.finding-list` + `.criterion` are the pair RunEvaluation's
                equivalent list uses; the rule is 套用既有的卡片，不是發明第五種
                樣式. */}
            <ul className="finding-list">
              {items.map((finding, index) => (
                <li className="criterion" key={`${finding.code}-${index}`}>
                  <FindingBody finding={finding} />
                </li>
              ))}
            </ul>
          </section>
        ) : null,
      )}
    </div>
  );
}

function FindingBody({ finding }: { finding: ImportFinding }) {
  return (
    <>
      {/* checklist 5 / §2.6: the sentence leads and the identifier follows it.
          `license-unknown` used to be the first thing on every line — an
          identifier ahead of the answer. Same shape as RunEvaluation's
          執行完成（succeeded） and the finding rows on Packaging. */}
      <p>
        {finding.message}（<span className="risk-code">{finding.code}</span>）
        {finding.path && (
          <>
            {" "}
            <code>{finding.path}</code>
          </>
        )}
      </p>
      {finding.details && finding.details.length > 0 && (
        <details>
          <summary>逐項列出（{finding.details.length} 筆）</summary>
          <ul className="risk-list">
            {finding.details.map((detail, index) => (
              <li key={`${detail}-${index}`}>{detail}</li>
            ))}
          </ul>
        </details>
      )}
    </>
  );
}
